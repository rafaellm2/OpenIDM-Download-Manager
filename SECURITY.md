# 🔒 Política de segurança / Security Policy

## Versões suportadas / Supported Versions

| Versão / Version | Suporte / Supported |
|---|---|
| 1.0.x | ✅ |

## 🇧🇷 Como relatar uma vulnerabilidade

**Não abra uma issue pública** para falhas de segurança.

Use o recurso [Report a vulnerability](../../security/advisories/new) (GitHub Security Advisories) deste repositório, informando:

- Versão do OpenIDM e do Windows;
- Componente afetado (motor, ponte local `127.0.0.1:41820`, Native Host, extensão, instalador);
- Passos para reproduzir e impacto esperado;
- Prova de conceito, se houver.

Você receberá uma resposta assim que possível. Após a correção, o relato será creditado no CHANGELOG (se desejar).

### Escopo

São especialmente relevantes:

- Acesso indevido à ponte HTTP local por páginas web ou outros processos;
- Execução de comandos via antivírus/ações pós-fila com argumentos não confiáveis;
- *Path traversal* em nomes de arquivos baixados ou no pacote do instalador;
- Exposição das credenciais salvas no banco local.

---

## 🇺🇸 Reporting a Vulnerability (English)

**Please do not open a public issue** for security vulnerabilities.

Instead, use this repository's [Report a vulnerability](../../security/advisories/new) (GitHub Security Advisories) page and provide:

- OpenIDM and Windows versions;
- Affected component (download engine, local HTTP bridge `127.0.0.1:41820`, Native Host, browser extension, or installer);
- Reproduction steps and potential impact;
- Proof of concept, if available.

### Verificando downloads / Verifying Release Checksums

Cada release publica um `SHA256SUMS.txt`. Confira antes de instalar / Verify before installing:

```powershell
Get-FileHash .\OpenIDM-Setup-1.0.1-x64.exe -Algorithm SHA256
```
