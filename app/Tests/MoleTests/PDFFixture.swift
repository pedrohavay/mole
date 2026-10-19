import Foundation

/// PDFFixture writes small PDF files for the tests.
enum PDFFixture {
    /// Returns a PDF of pages US Letter pages of text, with title and
    /// author. Each page has forty lines, which run down most of it.
    /// Encrypted, it has a security handler whose user password is not
    /// empty, which is all the core checks before refusing it.
    static func data(pages: Int = 1, encrypted: Bool = false) -> Data {
        var objects = [
            "<< /Type /Catalog /Pages 2 0 R >>",
            "", // the page tree, once the pages are numbered
            "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
            "<< /Title (Fixed Pages) /Author (Ada Burrow) >>",
        ]
        var kids: [String] = []
        for page in 0..<pages {
            let lines = (1...40).map { "(Line \($0) of page \(page + 1), set in Helvetica.) Tj T*" }
            let content = "BT /F1 12 Tf 14 TL 72 700 Td \(lines.joined(separator: " ")) ET"
            objects.append("<< /Length \(content.utf8.count) >>\nstream\n\(content)\nendstream")
            objects.append("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents \(objects.count) 0 R "
                + "/Resources << /Font << /F1 3 0 R >> >> >>")
            kids.append("\(objects.count) 0 R")
        }
        objects[1] = "<< /Type /Pages /Kids [\(kids.joined(separator: " "))] /Count \(pages) >>"
        let id = String(repeating: "ab", count: 16)
        var trailer = "/Root 1 0 R /Info 4 0 R /ID [<\(id)> <\(id)>]"
        if encrypted {
            let key = String(repeating: "5a", count: 32)
            objects.append("<< /Filter /Standard /V 1 /R 2 /O <\(key)> /U <\(key)> /P -4 >>")
            trailer += " /Encrypt \(objects.count) 0 R"
        }
        var file = "%PDF-1.4\n"
        var offsets: [Int] = []
        for (index, object) in objects.enumerated() {
            offsets.append(file.utf8.count)
            file += "\(index + 1) 0 obj\n\(object)\nendobj\n"
        }
        let xref = file.utf8.count
        file += "xref\n0 \(objects.count + 1)\n0000000000 65535 f \n"
        for offset in offsets { file += String(format: "%010d 00000 n \n", offset) }
        file += "trailer\n<< /Size \(objects.count + 1) \(trailer) >>\nstartxref\n\(xref)\n%%EOF\n"
        return Data(file.utf8)
    }
}
