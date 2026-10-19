# Changelog

Todas as mudanças relevantes do OpenIDM são registradas aqui. / All notable changes to OpenIDM are documented in this file.

O formato segue o [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/) e o projeto adota [Versionamento Semântico](https://semver.org/lang/pt-BR/).

## [1.0.0] — 2026-10-19

Primeira versão pública oficial / Initial official public release.

### 🌍 Internacionalização / Internationalization (PT-BR & EN-US)
- **Suporte oficial completo ao idioma Inglês (`en-US`) e Português (`pt-BR`)**:
  - Alternância instantânea de idioma na barra de ferramentas (`🌐 PT` / `🌐 EN`) e na janela de **Opções** (`Idioma da Interface / Interface Language`), com persistência automática no SQLite (`app_settings.language`).
  - Tradução completa de todas as telas do aplicativo desktop (Janela Principal, Card Solto, Salvar Novo Download, Detalhes e Barra de Segmentos, Agendador de Filas, Opções Avançadas e Assistente do Instalador).
  - Localização dinâmica do menu de contexto e *tooltips* do ícone na bandeja do Windows (*System Tray*), diálogos nativos de seleção de pasta e notificações de conclusão de download.
  - **Extensão do navegador (MV3)** bilíngue (`PT-BR` / `EN-US`), com sincronização automática do idioma configurado no OpenIDM Desktop via `/api/health`, botão de troca rápida no popup e menu de contexto traduzido ("Download with OpenIDM" / "Baixar com o OpenIDM").
  - **Instalador standalone (`OpenIDM-Setup-1.0.0-x64.exe`)** e guia interativo de ativação da extensão com suporte completo a Português e Inglês.
  - **Documentação bilíngue no GitHub**: adicionados `README.en.md`, `docs/USER-GUIDE.md`, `docs/BROWSER-EXTENSION.md`, `docs/ARCHITECTURE.md`, `docs/BUILDING.md` e templates bilíngues de Issues e Pull Requests.

### ⚡ Motor de download / Download Engine
- Segmentação dinâmica adaptativa com divisão do maior segmento em andamento.
- Reuso de conexões keep-alive com telemetria via `httptrace`.
- Até 32 conexões por download (padrão 8) e tamanho mínimo de divisão configurável.
- Checkpoints no SQLite e arquivo parcial `.oidmpart` para retomada robusta após falhas.
- Degradação automática para 1 conexão em servidores sem suporte a `Range`.
- Suporte a HTTP, HTTPS e FTP (PASV).
- Proxy HTTP/HTTPS, SOCKS5, proxy do sistema e PAC.
- Limitador de velocidade global e por download.
- Credenciais por servidor (Basic, Bearer, FTP).

### 🎯 Captura e Integração / Capture & Integration
- Extensão MV3 para Chrome, Edge, Brave e Firefox com menu de contexto.
- Tecla `ALT` para ignorar a captura instantaneamente.
- Native Messaging Host (`com.rafatech.openidm`) com registro automático no Windows Registry.
- Ponte HTTP local em `127.0.0.1:41820`.
- Monitor da área de transferência para links copiados.
- Exceções por site com suporte a caracteres curinga (`*.exemplo.com`).

### 📁 Organização e Agendador / Organization & Scheduler
- Categorias automáticas por extensão com pastas próprias (Comprimidos, Documentos, Música, Programas, Vídeo, Geral).
- Fila Principal, Fila de Sincronização e filas customizadas.
- Agendador com horários de início/parada, cota de tráfego e retries configuráveis.
- Comando pós-fila e desligamento automático do PC ao concluir.
- Verificação por antivírus externo ao concluir.

### 🖥️ Interface e Experiência / Desktop UI
- Interface escura moderna com lista de downloads, filtros, detalhes e barra de segmentos em tempo real.
- Card Solto flutuante compacto com posição persistida no banco.
- Ícone na bandeja do sistema (System Tray) com menu rápido e notificações nativas.
- Instalador gráfico por usuário com desinstalador limpo no Painel de Aplicativos do Windows.

### 📦 Distribuição / Distribution
- Instalador em arquivo único (`OpenIDM-Setup-1.0.0-x64.exe`) com o aplicativo, o Native Host e a extensão embutidos.
- Script `scripts/build.ps1` para compilação automatizada, empacotamento e geração da release com hash SHA-256.
