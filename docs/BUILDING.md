# 🛠️ Building & Development

🌐 **[🇧🇷 Leia em Português](COMPILACAO.md)** | **🇺🇸 English**

How to set up your development environment, compile OpenIDM from source, run the test suite, and package the release installer.

---

## Prerequisites

| Tool | Version | Notes |
|---|---|---|
| Windows | 10 or 11 (x64) | Target platform |
| [Go](https://go.dev/dl/) | 1.25+ | Backend engine and desktop host |
| [Node.js](https://nodejs.org/) | 18+ | Frontend build (`npm`) |
| PowerShell | 5.1+ or 7+ | Build automation script |
| [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/) | — | Included in Windows 10/11 |
| [go-winres](https://github.com/tc-hib/go-winres) | — | Automatically installed by `scripts/build.ps1` if missing |
| [Wails CLI](https://wails.io/docs/gettingstarted/installation) | v2.x | Optional, only needed for `wails dev` |

> No C/C++ compiler (CGO) is required: OpenIDM uses `modernc.org/sqlite` (100% pure Go).

---

## Full Build (Single Command)

```powershell
git clone https://github.com/rafaellm2/OpenIDM-Download-Manager.git
cd OpenIDM-Download-Manager
./scripts/build.ps1
```

[`scripts/build.ps1`](../scripts/build.ps1) executes the following pipeline:

| Step | Action |
|---|---|
| 1. Frontend | Runs `npm ci` (if needed) and `npm run build` → `frontend/dist/` |
| 2. Windows Resources | Uses `go-winres` to generate `.syso` resources (icons, manifest, version metadata) from `build/windows/winres.json` |
| 3. Verification | Runs `go vet ./...` and `go test ./...` |
| 4. Binaries | Compiles `openidm.exe` and `openidm-native-host.exe` into `build/bin/` |
| 5. Standalone Installer | Archives the binaries and `browser-extension/` into `payload.zip` and compiles `OpenIDM-Setup.exe` with `-tags embedpayload` |
| 6. Release Artifact | Copies the installer to `release/v<version>/OpenIDM-Setup-<version>-x64.exe` and generates `SHA256SUMS.txt` |

Optional script parameters:

| Parameter | Effect |
|---|---|
| `-SkipFrontend` | Reuses the existing `frontend/dist` build |
| `-SkipTests` | Skips `go vet` and `go test` |

The version number is read automatically from `wails.json` (`info.productVersion`).

---

## Running Tests

```powershell
# Run all tests across all packages
go test ./...

# Verbose output
go test -v ./...

# Test only the download engine
go test -v ./internal/engine/...
```

| Package | Test Coverage |
|---|---|
| `internal/engine` | Dynamic segmentation, largest-segment splitting, graceful degradation, resume, speed limiter |
| `internal/scheduler` | Queues, schedules, traffic quotas, retry logic, and ETag sync |
| `internal/store` | SQLite schema, repositories, language persistence, and migrations |
| `internal/integration` | URL classification, HTTP bridge, Native Messaging, bilingual installer setup |
| `internal/sysproc` | Process execution and command-line argument sanitization |
| `cmd/installer` | Embedded payload extraction and path-traversal protection |
| root (`app_test.go`) | End-to-end application workflows, capture deduplication, card mode, and language switching |

---

## Publishing a New Release

1. Update the version string in:
   - `wails.json` (`info.productVersion`)
   - `build/windows/winres.json`
   - `internal/integration/installer.go` (`AppVersion`)
   - `frontend/package.json`
   - `browser-extension/manifest.json`, `background.js`, `popup.js`
2. Document the changes in [CHANGELOG.md](../CHANGELOG.md).
3. Run `./scripts/build.ps1`.
4. Commit the generated `release/v<version>/` artifacts and create the Git tag:

   ```powershell
   git tag -a v1.0.0 -m "OpenIDM v1.0.0"
   git push origin main --tags
   ```

5. Publish the GitHub Release attaching the installer and `SHA256SUMS.txt`:

   ```powershell
   gh release create v1.0.0 release/v1.0.0/OpenIDM-Setup-1.0.0-x64.exe release/v1.0.0/SHA256SUMS.txt `
     --title "OpenIDM v1.0.0" --notes-file CHANGELOG.md
   ```
