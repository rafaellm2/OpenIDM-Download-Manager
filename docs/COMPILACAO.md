# 🛠️ Compilação e desenvolvimento

🌐 **🇧🇷 Português** | **[🇺🇸 Read in English](BUILDING.md)**

Como preparar o ambiente, compilar o OpenIDM, rodar os testes e gerar o instalador de release.

---

## Pré-requisitos

| Ferramenta | Versão | Observação |
|---|---|---|
| Windows | 10 ou 11 (x64) | Plataforma alvo |
| [Go](https://go.dev/dl/) | 1.25+ | Motor e app desktop |
| [Node.js](https://nodejs.org/) | 18+ | Build do frontend |
| PowerShell | 5.1+ ou 7+ | Script de build |
| [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/) | — | Já incluso no Windows 10/11 |
| [go-winres](https://github.com/tc-hib/go-winres) | — | Instalado automaticamente pelo script |
| [Wails CLI](https://wails.io/docs/gettingstarted/installation) | v2.x | Opcional, para `wails dev` |

> Não é necessário compilador C: o SQLite usado (`modernc.org/sqlite`) é Go puro.

---

## Build completo

```powershell
git clone <url-do-repositório> OpenIDM-Download-Manager
cd OpenIDM-Download-Manager
./scripts/build.ps1
```

O [`scripts/build.ps1`](../scripts/build.ps1) executa, em ordem:

| Etapa | O que faz |
|---|---|
| 1. Frontend | `npm ci` (se necessário) e `npm run build` → `frontend/dist/` |
| 2. Recursos do Windows | `go-winres` gera os `.syso` (ícone, manifesto e versão) a partir de `build/windows/winres.json` |
| 3. Testes | `go vet ./...` e `go test ./...` |
| 4. Executáveis | `openidm.exe` e `openidm-native-host.exe` em `build/bin/` |
| 5. Instalador | Compacta os executáveis e a extensão em `payload.zip` e compila `OpenIDM-Setup.exe` com eles embutidos |
| 6. Release | Copia para `release/v<versão>/OpenIDM-Setup-<versão>-x64.exe` e gera o `SHA256SUMS.txt` |

Parâmetros:

| Parâmetro | Efeito |
|---|---|
| `-SkipFrontend` | Reaproveita o `frontend/dist` já compilado |
| `-SkipTests` | Pula `go vet` e `go test` |

A versão vem de `wails.json` (`info.productVersion`).

---

## Build manual (passo a passo)

```powershell
# Frontend
cd frontend; npm ci; npm run build; cd ..

# Recursos do Windows (ícone e versão nos .exe)
go-winres make --in build/windows/winres.json --arch amd64 --out rsrc
go-winres make --in build/windows/winres.json --arch amd64 --out cmd/native-host/rsrc
go-winres make --in build/windows/winres.json --arch amd64 --out cmd/installer/rsrc

# Aplicativo e Native Host
go build -trimpath -tags "desktop,production" -ldflags "-s -w -H=windowsgui" -o build/bin/openidm.exe .
go build -trimpath -ldflags "-s -w -H=windowsgui" -o build/bin/openidm-native-host.exe ./cmd/native-host

# Instalador de desenvolvimento (usa os arquivos ao lado dele, sem nada embutido)
go build -trimpath -ldflags "-s -w -H=windowsgui" -o build/bin/OpenIDM-Setup.exe ./cmd/installer
```

| Flag | Efeito |
|---|---|
| `-tags "desktop,production"` | Build de produção do Wails (sem DevTools) |
| `-tags embedpayload` | Embute `cmd/installer/payload.zip` no instalador (usado pelo script de release) |
| `-trimpath` | Remove caminhos locais do binário |
| `-s -w` | Remove símbolos de depuração (binário menor) |
| `-H=windowsgui` | Aplicativo de janela, sem console |

### Como o instalador funciona

- **Release** (`-tags embedpayload`): o `OpenIDM-Setup.exe` carrega `openidm.exe`, `openidm-native-host.exe` e `browser-extension/` compactados dentro dele. Ao executar, extrai tudo para uma pasta temporária, instala e apaga os temporários. É um **arquivo único** para distribuir.
- **Desenvolvimento** (sem a tag): o instalador procura os arquivos na mesma pasta em que ele está (ou em `--source-dir`).

---

## Desenvolvimento com hot reload

```powershell
wails dev
```

O Wails sobe o Vite em modo de desenvolvimento e recarrega a interface a cada alteração em `frontend/src`.

Para testar sem afetar seus dados reais, use um banco separado:

```powershell
.\build\bin\openidm.exe --db "$env:TEMP\openidm-dev.sqlite"
```

> O `frontend/dist/` é embutido no `openidm.exe` via `//go:embed`. Ele não é versionado (só o `.gitkeep`); rode `npm run build` antes de compilar o app.

---

## Testes

```powershell
# Todos os testes
go test ./...

# Verboso
go test -v ./...

# Apenas o motor
go test -v ./internal/engine/...
```

| Pacote | O que é testado |
|---|---|
| `internal/engine` | Segmentação dinâmica, divisão do maior segmento, degradação, retomada, limitador |
| `internal/scheduler` | Filas, horários, cotas, retries e sincronização |
| `internal/store` | Esquema, repositórios e migrações (incluindo casos adversariais) |
| `internal/integration` | Classificação, ponte HTTP, Native Messaging, instalação |
| `internal/sysproc` | Execução de processos e tratamento de argumentos |
| `cmd/installer` | Extração do pacote embutido e proteção contra *path traversal* |
| raiz (`app_test.go`) | Fluxos da camada de aplicação e suporte a idiomas |

---

## Extensão do navegador

A extensão em `browser-extension/` não precisa de build — é JavaScript puro (MV3). Para testar, carregue a pasta sem compactação conforme [EXTENSAO-NAVEGADOR.md](EXTENSAO-NAVEGADOR.md).

---

## Publicando uma nova versão

1. Atualize a versão em:
   - `wails.json` (`info.productVersion`)
   - `build/windows/winres.json`
   - `internal/integration/installer.go` (`AppVersion`)
   - `frontend/package.json`
   - `browser-extension/manifest.json`
2. Registre as mudanças no [CHANGELOG](../CHANGELOG.md).
3. Rode `./scripts/build.ps1`.
4. Faça o commit da pasta `release/v<versão>/` e crie a tag:

   ```powershell
   git tag -a v1.0.0 -m "OpenIDM v1.0.0"
   git push origin main --tags
   ```

5. Crie a release no GitHub anexando o instalador e o `SHA256SUMS.txt`:

   ```powershell
   gh release create v1.0.0 release/v1.0.0/OpenIDM-Setup-1.0.0-x64.exe release/v1.0.0/SHA256SUMS.txt `
     --title "OpenIDM v1.0.0" --notes-file CHANGELOG.md
   ```
