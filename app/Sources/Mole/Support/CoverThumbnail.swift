import AppKit
import ImageIO

/// CoverThumbnail decodes cover images at the size the library shows them,
/// so a cover with huge pixel dimensions never decodes in full.
enum CoverThumbnail {
    /// MaxPixels is the longest side of a decoded cover: the grid cell is
    /// about 225 points tall, at two pixels per point, with some margin.
    static let maxPixels = 640

    /// Returns a thumbnail of the image in data, or nil if it is not one.
    /// It is safe to call from any thread.
    static func image(from data: Data) -> CGImage? {
        guard let source = CGImageSourceCreateWithData(data as CFData, nil) else { return nil }
        let options: [CFString: Any] = [
            kCGImageSourceCreateThumbnailFromImageAlways: true,
            kCGImageSourceCreateThumbnailWithTransform: true,
            kCGImageSourceShouldCacheImmediately: true,
            kCGImageSourceThumbnailMaxPixelSize: maxPixels,
        ]
        return CGImageSourceCreateThumbnailAtIndex(source, 0, options as CFDictionary)
    }
}
