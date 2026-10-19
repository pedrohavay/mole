// ed25519-verify checks an EdDSA (Ed25519) signature the way Sparkle does
// before it installs an update. release.sh runs it on the signed zip with
// the public key built into the app, so a release never ships an update
// that installed copies would reject.
//
// Usage: xcrun swift scripts/ed25519-verify.swift PUBLIC_KEY SIGNATURE FILE
//
// PUBLIC_KEY and SIGNATURE are base64, as in Info.plist and the appcast. It
// exits with status 0 when SIGNATURE is FILE's signature by PUBLIC_KEY.
import CryptoKit
import Foundation

func fail(_ message: String) -> Never {
    FileHandle.standardError.write(Data("ed25519-verify: \(message)\n".utf8))
    exit(1)
}

let arguments = CommandLine.arguments.dropFirst()
guard arguments.count == 3 else { fail("usage: ed25519-verify.swift PUBLIC_KEY SIGNATURE FILE") }
let values = Array(arguments)
guard let keyData = Data(base64Encoded: values[0]),
      let key = try? Curve25519.Signing.PublicKey(rawRepresentation: keyData) else { fail("the public key is not valid") }
guard let signature = Data(base64Encoded: values[1]) else { fail("the signature is not base64") }
guard let file = FileManager.default.contents(atPath: values[2]) else { fail("cannot read \(values[2])") }
guard key.isValidSignature(signature, for: file) else { fail("the signature does not match the public key") }
