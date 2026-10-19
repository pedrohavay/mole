import AppKit
import Metal
import QuartzCore

/// Mark is a rectangle painted under the text, such as a highlight, the
/// selection or a search hit, in drawable pixels from the top left.
struct Mark: Equatable {
    var rect: CGRect
    /// Color is the light of the mark in linear Display P3, kept dim so the
    /// text above it stays readable.
    var color: SIMD3<Float>
}

/// EDRPipeline is the compiled Metal pipeline, shared by every reader view.
/// It is built once; when Metal is missing, the failure explains why.
@MainActor
enum EDRPipeline {
    static let shared: Result<Pipeline, Error> = Result { try Pipeline() }

    /// Pipeline holds the Metal objects that do not depend on a view.
    struct Pipeline {
        let device: MTLDevice
        let queue: MTLCommandQueue
        let text: MTLRenderPipelineState
        let marks: MTLRenderPipelineState
        /// Blank is a 1x1 empty texture, bound for a layer the page lacks.
        let blank: MTLTexture

        init() throws {
            guard let device = MTLCreateSystemDefaultDevice(), let queue = device.makeCommandQueue() else {
                throw CoreError(message: L10n.text("error.metal"))
            }
            let library = try device.makeLibrary(source: EDRPipeline.source, options: nil)

            let text = MTLRenderPipelineDescriptor()
            text.vertexFunction = library.makeFunction(name: "vs_fullscreen")
            text.fragmentFunction = library.makeFunction(name: "fs_text")
            text.colorAttachments[0].pixelFormat = .rgba16Float
            // The text goes over the marks: out = text + marks * (1 - coverage).
            text.colorAttachments[0].isBlendingEnabled = true
            text.colorAttachments[0].sourceRGBBlendFactor = .one
            text.colorAttachments[0].destinationRGBBlendFactor = .oneMinusSourceAlpha
            text.colorAttachments[0].sourceAlphaBlendFactor = .one
            text.colorAttachments[0].destinationAlphaBlendFactor = .zero

            let marks = MTLRenderPipelineDescriptor()
            marks.vertexFunction = library.makeFunction(name: "vs_mark")
            marks.fragmentFunction = library.makeFunction(name: "fs_mark")
            marks.colorAttachments[0].pixelFormat = .rgba16Float

            let blankDescriptor = MTLTextureDescriptor.texture2DDescriptor(
                pixelFormat: .rgba8Unorm, width: 1, height: 1, mipmapped: false)
            guard let blank = device.makeTexture(descriptor: blankDescriptor) else {
                throw CoreError(message: L10n.text("error.metal"))
            }
            var zero: UInt32 = 0
            blank.replace(region: MTLRegionMake2D(0, 0, 1, 1), mipmapLevel: 0, withBytes: &zero, bytesPerRow: 4)

            self.device = device
            self.queue = queue
            self.text = try device.makeRenderPipelineState(descriptor: text)
            self.marks = try device.makeRenderPipelineState(descriptor: marks)
            self.blank = blank
        }
    }

    nonisolated static let source = """
    #include <metal_stdlib>
    using namespace metal;

    struct VertexOut { float4 position [[position]]; };

    // One triangle that covers the whole viewport; no vertex buffer needed.
    vertex VertexOut vs_fullscreen(uint vid [[vertex_id]]) {
        float2 p = float2(float((vid << 1) & 2), float(vid & 2));
        VertexOut out;
        out.position = float4(p * 2.0 - 1.0, 0.0, 1.0);
        return out;
    }

    struct TextParams { float4 tint; float gain; float imageGain; float gamma; };

    // Returns an sRGB color in linear Display P3, the space of the layer.
    float3 linear_p3(float3 srgb) {
        float3 linear = select(pow((srgb + 0.055) / 1.055, 2.4), srgb / 12.92, srgb <= 0.04045);
        const float3x3 toP3 = float3x3(float3(0.8225, 0.0332, 0.0171),
                                       float3(0.1774, 0.9669, 0.0724),
                                       float3(0.0, 0.0, 0.9108));
        return toP3 * linear;
    }

    // Text light is ink * gain * coverage^gamma, where ink is the tint, or
    // the text's own hue where the colors layer has one. Pictures lie under
    // the text, as they are on black times imageGain, which stays at or
    // below SDR white. Alpha carries what covers the pixel, for blending
    // over marks.
    fragment float4 fs_text(VertexOut in [[stage_in]],
                            texture2d<float, access::read> text [[texture(0)]],
                            texture2d<float, access::read> colors [[texture(1)]],
                            texture2d<float, access::read> image [[texture(2)]],
                            constant TextParams& p [[buffer(0)]]) {
        uint2 size = uint2(text.get_width(), text.get_height());
        uint2 xy = min(uint2(in.position.xy), size - 1);
        float raw = text.read(xy).r;
        float coverage = pow(raw, p.gamma);
        float3 ink = p.tint.rgb;
        if (colors.get_width() == size.x && colors.get_height() == size.y) {
            float4 hue = colors.read(xy);
            if (hue.a > 0.0) {
                float share = saturate(hue.a / max(raw, 1.0 / 255.0));
                ink = mix(ink, linear_p3(hue.rgb / hue.a), share);
            }
        }
        float3 light = ink * (p.gain * coverage);
        float alpha = coverage;
        if (image.get_width() == size.x && image.get_height() == size.y) {
            float4 picture = image.read(xy);
            if (picture.a > 0.0) {
                light += linear_p3(picture.rgb / picture.a) * (picture.a * p.imageGain * (1.0 - coverage));
                alpha += picture.a * (1.0 - coverage);
            }
        }
        return float4(light, alpha);
    }

    struct MarkIn { float4 rect; float4 color; };
    struct MarkOut { float4 position [[position]]; float4 color; };

    // Each mark is an instance of a two-triangle quad; rect is x, y, width,
    // height in pixels from the top left of a drawable of size viewport.
    vertex MarkOut vs_mark(uint vid [[vertex_id]], uint iid [[instance_id]],
                           constant MarkIn* marks [[buffer(0)]],
                           constant float2& viewport [[buffer(1)]]) {
        const float2 corners[6] = { {0, 0}, {1, 0}, {0, 1}, {1, 0}, {1, 1}, {0, 1} };
        MarkIn mark = marks[iid];
        float2 pixel = mark.rect.xy + corners[vid] * mark.rect.zw;
        float2 ndc = float2(pixel.x / viewport.x * 2.0 - 1.0, 1.0 - pixel.y / viewport.y * 2.0);
        MarkOut out;
        out.position = float4(ndc, 0.0, 1.0);
        out.color = mark.color;
        return out;
    }

    fragment float4 fs_mark(MarkOut in [[stage_in]]) {
        return float4(in.color.rgb, 1.0);
    }
    """
}

/// EDRRenderer turns the layers of a session into light on an EDR layer:
/// background pixels are written as 0.0 and glyph pixels as
/// ink * coverage^gamma, where ink may be above 1.0, brighter than SDR white.
/// Pictures stay at or below SDR white.
@MainActor
final class EDRRenderer {
    private let pipeline: EDRPipeline.Pipeline
    private var text: MTLTexture?
    private var colors: MTLTexture?
    private var image: MTLTexture?

    private struct TextParams {
        var tint: SIMD4<Float>
        var gain: Float
        var imageGain: Float
        var gamma: Float
        var padding: Float = 0
    }

    private struct MarkParams {
        var rect: SIMD4<Float>
        var color: SIMD4<Float>
    }

    init() throws {
        pipeline = try EDRPipeline.shared.get()
    }

    /// Returns a layer configured for Extended Dynamic Range content.
    static func makeLayer(opaque: Bool = true) -> CAMetalLayer {
        let layer = CAMetalLayer()
        layer.device = (try? EDRPipeline.shared.get())?.device ?? MTLCreateSystemDefaultDevice()
        layer.pixelFormat = .rgba16Float
        layer.wantsExtendedDynamicRangeContent = true
        layer.colorspace = CGColorSpace(name: CGColorSpace.extendedLinearDisplayP3)
        layer.framebufferOnly = true
        layer.isOpaque = opaque
        return layer
    }

    /// Copies the text mask into its texture.
    func upload(text mask: PixelLayer) {
        text = upload(mask, format: .r8Unorm, into: text)
    }

    /// Copies the hues of colored text into their texture, or drops it when
    /// there is none.
    func upload(colors layer: PixelLayer?) {
        colors = layer.flatMap { upload($0, format: .rgba8Unorm, into: colors) }
    }

    /// Copies the pictures into their texture, or drops it when there is none.
    func upload(image layer: PixelLayer?) {
        image = layer.flatMap { upload($0, format: .rgba8Unorm, into: image) }
    }

    private func upload(_ layer: PixelLayer, format: MTLPixelFormat, into texture: MTLTexture?) -> MTLTexture? {
        var texture = texture
        if texture?.width != layer.width || texture?.height != layer.height {
            let descriptor = MTLTextureDescriptor.texture2DDescriptor(
                pixelFormat: format, width: layer.width, height: layer.height, mipmapped: false)
            descriptor.usage = .shaderRead
            texture = pipeline.device.makeTexture(descriptor: descriptor)
        }
        texture?.replace(region: MTLRegionMake2D(0, 0, layer.width, layer.height), mipmapLevel: 0,
                         withBytes: layer.bytes, bytesPerRow: layer.stride)
        return texture
    }

    /// Draws into the layer's next drawable: marks first, then the pictures
    /// with their colors * imageGain and the text with tint (or its own hue)
    /// * gain over them.
    func present(on layer: CAMetalLayer, tint: SIMD3<Float>, gain: Double, gamma: Double,
                 imageGain: Double = 0, marks: [Mark] = []) {
        guard let text, let drawable = layer.nextDrawable(),
              let commands = pipeline.queue.makeCommandBuffer() else { return }
        let pass = MTLRenderPassDescriptor()
        pass.colorAttachments[0].texture = drawable.texture
        pass.colorAttachments[0].loadAction = .clear
        pass.colorAttachments[0].clearColor = MTLClearColor(red: 0, green: 0, blue: 0, alpha: 1)
        pass.colorAttachments[0].storeAction = .store
        guard let encoder = commands.makeRenderCommandEncoder(descriptor: pass) else { return }

        if !marks.isEmpty {
            var params = marks.map { mark in
                MarkParams(rect: SIMD4(Float(mark.rect.minX), Float(mark.rect.minY),
                                       Float(mark.rect.width), Float(mark.rect.height)),
                           color: SIMD4(mark.color, 1))
            }
            var viewport = SIMD2(Float(drawable.texture.width), Float(drawable.texture.height))
            encoder.setRenderPipelineState(pipeline.marks)
            encoder.setVertexBytes(&params, length: MemoryLayout<MarkParams>.stride * params.count, index: 0)
            encoder.setVertexBytes(&viewport, length: MemoryLayout<SIMD2<Float>>.stride, index: 1)
            encoder.drawPrimitives(type: .triangle, vertexStart: 0, vertexCount: 6, instanceCount: params.count)
        }

        var params = TextParams(tint: SIMD4(tint, 0), gain: Float(gain), imageGain: Float(imageGain),
                                gamma: Float(gamma))
        encoder.setRenderPipelineState(pipeline.text)
        encoder.setFragmentTexture(text, index: 0)
        encoder.setFragmentTexture(colors ?? pipeline.blank, index: 1)
        encoder.setFragmentTexture(image ?? pipeline.blank, index: 2)
        encoder.setFragmentBytes(&params, length: MemoryLayout<TextParams>.stride, index: 0)
        encoder.drawPrimitives(type: .triangle, vertexStart: 0, vertexCount: 3)
        encoder.endEncoding()
        commands.present(drawable)
        commands.commit()
        // Frames are drawn on demand; waiting keeps the next upload from
        // racing the GPU without double-buffering the textures.
        commands.waitUntilCompleted()
    }

    /// Presents one fully transparent frame, which is enough for macOS to
    /// raise the EDR headroom while the layer is on screen.
    static func presentClear(on layer: CAMetalLayer) {
        guard let pipeline = try? EDRPipeline.shared.get(), let drawable = layer.nextDrawable(),
              let commands = pipeline.queue.makeCommandBuffer() else { return }
        let pass = MTLRenderPassDescriptor()
        pass.colorAttachments[0].texture = drawable.texture
        pass.colorAttachments[0].loadAction = .clear
        pass.colorAttachments[0].clearColor = MTLClearColor(red: 0, green: 0, blue: 0, alpha: 0)
        pass.colorAttachments[0].storeAction = .store
        commands.makeRenderCommandEncoder(descriptor: pass)?.endEncoding()
        commands.present(drawable)
        commands.commit()
    }
}
