# 🤝 Contribuindo com o OpenIDM / Contributing to OpenIDM

🌐 **[🇧🇷 Português](#-como-reportar-um-bug)** | **[🇺🇸 English](#-english--contributing-guide)**

Obrigado pelo interesse em contribuir! Toda ajuda é bem-vinda: código, testes, documentação, traduções, design e relatos de bugs.

## Como reportar um bug

Abra uma *issue* com:

- Versão do OpenIDM e do Windows;
- Navegador e versão (se envolver a extensão);
- Passos para reproduzir;
- Comportamento esperado e o que aconteceu;
- URL de teste (se for pública) e capturas de tela.

> Não publique URLs privadas, tokens ou credenciais nas issues.

## Como sugerir uma funcionalidade

Abra uma *issue* descrevendo o problema que a funcionalidade resolve, como você imagina o uso e alternativas consideradas. Confira antes o [Roadmap](README.md).

## Fluxo de desenvolvimento

1. Faça um *fork* e clone o repositório.
2. Prepare o ambiente conforme [docs/COMPILACAO.md](docs/COMPILACAO.md).
3. Crie uma branch a partir da `main`:

   ```powershell
   git checkout -b feature/nome-da-melhoria
   ```

4. Faça suas alterações com testes.
5. Garanta que tudo passa:

   ```powershell
   go vet ./...
   go test ./...
   ./scripts/build.ps1   # build completo, opcional
   ```

6. Abra um *Pull Request* descrevendo o que mudou e por quê.

## Padrões

### Commits

Usamos [Conventional Commits](https://www.conventionalcommits.org/pt-br/):

| Prefixo | Uso |
|---|---|
| `feat:` | Nova funcionalidade |
| `fix:` | Correção de bug |
| `docs:` | Documentação |
| `refactor:` | Refatoração sem mudança de comportamento |
| `test:` | Testes |
| `perf:` | Desempenho |
| `chore:` | Build, dependências, tarefas gerais |

### Código

- **Go** — formatado com `gofmt` nos arquivos alterados, sem avisos do `go vet`.
- **Frontend** — TypeScript estrito; componentes React funcionais em `frontend/src/components`; textos de interface em `frontend/src/i18n/index.tsx` (`pt-BR` e `en-US`).
- **Testes** — toda mudança no motor (`internal/engine`), agendador ou armazenamento deve vir com testes.
- **Dependências** — prefira a biblioteca padrão; evite adicionar dependências com CGO.

### Arquivos que não vão para o repositório

Saídas de build (`build/bin/`, `frontend/dist/`, `*.syso`, `.exe`), `node_modules/` e bancos `.sqlite` são ignorados pelo `.gitignore`. A única exceção são os instaladores oficiais em `release/v<versão>/`, gerados pelo `scripts/build.ps1`.

### Reimplementação limpa

O OpenIDM é uma implementação independente. **Não envie** código, binários, ícones, textos ou outros recursos copiados do *Internet Download Manager* ou de qualquer software proprietário.

---

## 🇺🇸 English — Contributing Guide

Thank you for your interest in contributing to OpenIDM! Contributions of all kinds are welcome: code, tests, documentation, translations, UI polish, and bug reports.

### Reporting a Bug
Open an issue including your OpenIDM version, Windows version, browser version (if related to the extension), steps to reproduce, expected vs. actual behavior, and public test URLs if applicable. Never post private URLs, tokens, or credentials.

### Development Workflow
1. Fork and clone the repository.
2. Set up your environment following [docs/BUILDING.md](docs/BUILDING.md).
3. Create a branch from `main` (`git checkout -b feature/my-feature`).
4. Implement your changes and keep both `pt-BR` and `en-US` translations in sync (`frontend/src/i18n/index.tsx`).
5. Verify everything passes (`go vet ./...`, `go test ./...`, and optionally `./scripts/build.ps1`).
6. Open a Pull Request describing what changed and why.

## Licença / License

Ao contribuir, você concorda que sua contribuição será licenciada sob a [Licença MIT](LICENSE) do projeto. / By contributing, you agree that your work will be licensed under the project's [MIT License](LICENSE).
