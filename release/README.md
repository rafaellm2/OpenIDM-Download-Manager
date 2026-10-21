# 📦 Releases do OpenIDM / OpenIDM Releases

Instaladores oficiais, organizados por versão. Cada pasta contém o instalador standalone (x64) e o arquivo de verificação SHA-256.  
Official standalone installers organized by version. Each folder includes the x64 installer and its SHA-256 checksum file.

| Versão / Version | Data / Date | Instalador / Installer | Notas / Notes |
|---|---|---|---|
| **1.0.1** *(Latest)* | 2026-10-21 | [OpenIDM-Setup-1.0.1-x64.exe](v1.0.1/OpenIDM-Setup-1.0.1-x64.exe) | [Changelog — Atualizador Integrado (GitHub Releases) & Filtros de Captura](../CHANGELOG.md#101--2026-10-21) |
| **1.0.0** | 2026-10-19 | [OpenIDM-Setup-1.0.0-x64.exe](v1.0.0/OpenIDM-Setup-1.0.0-x64.exe) | [Changelog — Lançamento Oficial Bilíngue (PT-BR & EN-US)](../CHANGELOG.md#100--2026-10-19) |

## Requisitos / Requirements

- Windows 10 ou 11 (64 bits / x64)
- Microsoft Edge WebView2 Runtime (já incluso no Windows 10/11 / included in Windows 10/11)
- Não exige privilégios de administrador / Does not require administrator privileges

## Instalação / Installation

1. Baixe o `OpenIDM-Setup-1.0.1-x64.exe` / Download `OpenIDM-Setup-1.0.1-x64.exe`.
2. Verifique a integridade (opcional) / Verify SHA-256 checksum (optional):

   ```powershell
   Get-FileHash .\OpenIDM-Setup-1.0.1-x64.exe -Algorithm SHA256
   ```

   O resultado deve ser igual ao valor em `SHA256SUMS.txt` / The hash must match `SHA256SUMS.txt`.
3. Execute o instalador e siga o assistente (disponível em **Português** e **Inglês**) / Run the installer and follow the setup wizard (available in **English** and **Portuguese**).

Instalação silenciosa / Silent installation:

```powershell
.\OpenIDM-Setup-1.0.1-x64.exe --silent
.\OpenIDM-Setup-1.0.1-x64.exe --silent --install-dir "D:\Apps\OpenIDM" --launch-app
```

Desinstalação / Uninstallation: **Configurações → Aplicativos → OpenIDM → Desinstalar** (Windows Installed Apps), ou `openidm.exe --uninstall`.

## Gerando uma nova release / Building a New Release

```powershell
./scripts/build.ps1
```

O instalador é gravado em `release/v<versão>/`. Veja [docs/COMPILACAO.md](../docs/COMPILACAO.md) / [docs/BUILDING.md](../docs/BUILDING.md).
