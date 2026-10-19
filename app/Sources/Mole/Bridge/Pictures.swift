import CoreGraphics
import Foundation
import MoleCore

/// PagePicture is an image on the visible pages, as the core finds it under a
/// point: the page that draws it, its place among that page's images and
/// where it is on screen, in view points from the top left.
struct PagePicture: Decodable, Equatable {
    let page: Int
    let index: Int
    let rect: CGRect

    private enum CodingKeys: String, CodingKey { case page, index, rect }
    private struct Rect: Decodable { let x, y, width, height: Double }

    init(page: Int, index: Int, rect: CGRect) {
        self.page = page
        self.index = index
        self.rect = rect
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        page = try values.decode(Int.self, forKey: .page)
        index = try values.decode(Int.self, forKey: .index)
        let rect = try values.decode(Rect.self, forKey: .rect)
        self.rect = CGRect(x: rect.x, y: rect.y, width: rect.width, height: rect.height)
    }
}

extension Session {
    /// Returns the picture at point, in view points from the top left, or
    /// nil when there is none.
    func picture(at point: CGPoint) throws -> PagePicture? {
        try checkOpen()
        return try CoreCall.decode(PagePicture?.self, mole_session_image_at(handle, point.x, point.y))
    }

    /// Draws picture as it looks on a white page, in its colors and at its
    /// own resolution.
    func image(of picture: PagePicture) throws -> CGImage {
        try checkOpen()
        var width: Int64 = 0, height: Int64 = 0
        guard let pixels = mole_session_image_pixels(handle, Int64(picture.page), Int64(picture.index),
                                                     &width, &height) else {
            throw CoreCall.lastError()
        }
        let data = Data(bytesNoCopy: pixels, count: Int(width * height * 4), deallocator: .custom { bytes, _ in
            mole_free(bytes)
        })
        guard let provider = CGDataProvider(data: data as CFData),
              let space = CGColorSpace(name: CGColorSpace.sRGB),
              let image = CGImage(width: Int(width), height: Int(height), bitsPerComponent: 8, bitsPerPixel: 32,
                                  bytesPerRow: Int(width) * 4, space: space,
                                  bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.noneSkipLast.rawValue),
                                  provider: provider, decode: nil, shouldInterpolate: true, intent: .defaultIntent)
        else { throw CoreError(message: L10n.text("error.picture")) }
        return image
    }
}
