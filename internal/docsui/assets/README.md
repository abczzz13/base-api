# Swagger UI assets

This directory vendors frontend assets from `swagger-ui-dist`.

- Source package: `swagger-ui-dist@5.17.14`
- Source tarball: `https://registry.npmjs.org/swagger-ui-dist/-/swagger-ui-dist-5.17.14.tgz`
- License: Apache-2.0

## Update process

1. Download the new `swagger-ui-dist` tarball from npm.
2. Replace:
   - `swagger-ui.css`
   - `swagger-ui-bundle.js`
   - `swagger-ui-standalone-preset.js`
3. Update this file with the new version and tarball URL.
4. Refresh `swagger-ui-bundle.js.LICENSE.txt` and `swagger-ui-standalone-preset.js.LICENSE.txt` from upstream notices for the new bundles.
5. Run `go test ./...`.
