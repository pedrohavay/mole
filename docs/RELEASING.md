# Releasing Mole

This page is for the maintainer. It covers the keys a release needs, how to
publish one from a tag, and how to build one on a Mac.

## What the release workflow does

Pushing a version tag starts `.github/workflows/release.yml`. The workflow:

1. Checks that the tag matches `VERSION` in the Makefile.
2. Builds Mole against the pinned MuPDF.
3. Signs the app, and the Sparkle framework inside it, with a Developer ID
   and the hardened runtime.
4. Starts the signed app once to check that it runs.
5. Notarizes the app and staples the ticket.
6. Packs the app into `Mole.dmg`, with a link to Applications, then signs,
   notarizes and staples the disk image too.
7. Packs the app into `Mole-<version>.zip` and signs it for Sparkle.
8. Publishes a GitHub release named after the tag, with `Mole.dmg`,
   `Mole-<version>.zip`, their SHA-256 and `appcast.xml`.

`Mole.dmg` is what people download. It keeps the same name in every
release, so the README links the latest one directly:
`https://github.com/pedrohavay/mole/releases/latest/download/Mole.dmg`.
The zip is what Sparkle installs when an installed copy updates.

The release notes link the tagged source code, as the AGPL asks. Installed
copies read the appcast of the latest release to find new versions:
`https://github.com/pedrohavay/mole/releases/latest/download/appcast.xml`.

## Setting up the keys

You do this once. The first three steps need an Apple Developer Program
membership.

> [!CAUTION]
> Keep a backup of the Sparkle private key from step 4. Installed copies only
> accept updates signed with it. If you lose it, users must download the next
> version by hand.

1. Create the "Developer ID Application" certificate. In Xcode, open
   Settings > Accounts, select the team, click Manage Certificates, then +
   and Developer ID Application. Only the account holder can create it.

2. Export the certificate with its private key. In Keychain Access, open the
   login keychain and My Certificates. Select "Developer ID Application:
   Your Name (TEAMID)" and check that a private key sits under it. Choose
   File > Export Items, save it as a .p12 file, and set a password. Then
   encode it for GitHub:

   ```bash
   base64 -i DeveloperID.p12 -o DeveloperID.p12.base64
   ```

3. Create an App Store Connect API key for notarization. In App Store
   Connect, open Users and Access > Integrations > App Store Connect API.
   The account holder asks for access the first time. Under Team Keys, make
   a key with the Developer role. Download `AuthKey_<key ID>.p8`, which you
   can only download once. Write down the key ID and the issuer ID shown
   above the list of keys.

4. Create the update keys. `make app` downloads Sparkle, whose tools are in
   `app/.build/artifacts/sparkle/Sparkle/bin`. `generate_keys` makes a key
   pair, keeps the private key in the login keychain and prints the public
   key. `generate_keys -p` prints it again later. Export the private key to
   a file for GitHub:

   ```bash
   app/.build/artifacts/sparkle/Sparkle/bin/generate_keys
   ```

   ```bash
   app/.build/artifacts/sparkle/Sparkle/bin/generate_keys -x sparkle_private_key
   ```

5. Add the secrets and the variable to the repository, under Settings >
   Secrets and variables > Actions, or with `gh`:

   | Name | Kind | Value |
   |------|------|-------|
   | `DEVELOPER_ID_P12` | Secret | The contents of `DeveloperID.p12.base64` |
   | `DEVELOPER_ID_P12_PASSWORD` | Secret | The password of the .p12 file |
   | `NOTARY_KEY_P8` | Secret | The contents of `AuthKey_<key ID>.p8` |
   | `NOTARY_KEY_ID` | Secret | The key ID |
   | `NOTARY_ISSUER` | Secret | The issuer ID |
   | `SPARKLE_PRIVATE_KEY` | Secret | The contents of `sparkle_private_key` |
   | `SPARKLE_PUBLIC_KEY` | Variable | The public key from `generate_keys -p` |

   ```bash
   gh secret set DEVELOPER_ID_P12 < DeveloperID.p12.base64
   ```

   ```bash
   gh variable set SPARKLE_PUBLIC_KEY --body "$(app/.build/artifacts/sparkle/Sparkle/bin/generate_keys -p)"
   ```

   Then delete the exported files: `DeveloperID.p12`, its base64 copy,
   `sparkle_private_key`, and the .p8 file once it is stored somewhere safe.

## Publishing a release

Set `VERSION` in the Makefile to the new version and commit it on main.
Then tag that commit and push the tag:

```bash
git tag -a v0.2.0 -m "Mole 0.2.0"
```

```bash
git push origin v0.2.0
```

> [!IMPORTANT]
> The workflow refuses a tag that does not match `VERSION`. Sparkle compares
> build numbers, and the build number is the count of commits. Each release
> must come from a later commit on main than the one before.

## Building a release on a Mac

`make release` does the same work on a Mac that has the certificate in its
keychain, but it publishes nothing. For notarization it takes one of two
things:

- A notarytool keychain profile in `NOTARY_PROFILE`.
- The API key itself: the path of the .p8 file in `NOTARY_KEY`, plus
  `NOTARY_KEY_ID` and `NOTARY_ISSUER`.

To store a profile once and release with it:

```bash
xcrun notarytool store-credentials mole --key AuthKey_KEYID.p8 --key-id KEYID --issuer ISSUER_ID
```

```bash
make release SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" NOTARY_PROFILE=mole SPARKLE_PUBLIC_KEY=PUBLIC_KEY SPARKLE_KEY_FILE=sparkle_private_key
```

`SPARKLE_KEY_FILE` is a private key exported with `generate_keys -x`. The
`SPARKLE_PRIVATE_KEY` environment variable can hold the key instead. Leave
out both Sparkle keys to build a release with the updater off.

The script checks the credentials before it builds anything. It also checks
that the zip's signature matches the public key inside the app. Upload
`Mole.dmg`, the zip, their `.sha256` files and `appcast.xml` to a GitHub
release of the tag `v<version>`.

The first `make release` creates a Python virtual environment in
`build/dmgbuild` and installs a pinned dmgbuild there. It needs Python 3
and a network connection that one time.

## Dry runs and the updater in development

`make release SIGN_IDENTITY=- NOTARIZE=no` is a dry run that needs no Apple
account. CI runs it on every push. An ad hoc signature has no team, so the
hardened runtime would refuse to load Sparkle. Only in this case, the app
gets the `com.apple.security.cs.disable-library-validation` entitlement.

Development and CI builds have no `SPARKLE_PUBLIC_KEY`, so their updater is
off and the Check for Updates item is hidden. To try the updater, pass a
public key, for example `make run SPARKLE_PUBLIC_KEY=PUBLIC_KEY`. Set
`SPARKLE_FEED_URL` to read another appcast.
