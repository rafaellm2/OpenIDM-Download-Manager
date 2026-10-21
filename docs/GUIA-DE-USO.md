# 📘 Guia de uso

🌐 **🇧🇷 Português** | **[🇺🇸 Read in English](USER-GUIDE.md)**

Este guia apresenta a interface do OpenIDM e as tarefas do dia a dia.

- [Alternando o idioma (Português / Inglês)](#alternando-o-idioma-português--inglês)
- [Janela principal](#janela-principal)
- [Adicionando downloads](#adicionando-downloads)
- [Acompanhando um download](#acompanhando-um-download)
- [Card Solto](#card-solto)
- [Bandeja do sistema](#bandeja-do-sistema)
- [Categorias](#categorias)
- [Filas e agendador](#filas-e-agendador)
- [Opções](#opções)
- [Atalhos e dicas](#atalhos-e-dicas)

---

## Alternando o idioma (Português / Inglês)

O OpenIDM conta com suporte nativo a **Português (`pt-BR`)** e **Inglês (`en-US`)**:

1. **Botão rápido na Barra de Ferramentas** — clique em **`🌐 PT`** / **`🌐 EN`** no canto superior direito da janela principal para alternar o idioma instantaneamente.
2. **Janela de Opções** — abra **Opções → Conexão e Exceções por Site** e escolha o **Idioma da Interface** (`🇧🇷 Português (Brasil)` ou `🇺🇸 English (United States)`).
3. **Extensão do Navegador e Instalador** — tanto o popup da extensão quanto o instalador oficial (`OpenIDM-Setup.exe`) permitem alternar entre `PT` e `EN` no cabeçalho e sincronizam automaticamente com o aplicativo.

---

## Janela principal

![Janela principal](images/tela-principal.png)

| Área | O que faz |
|---|---|
| **Barra de ferramentas** | Capturar Link, Retomar, Pausar, Parar Tudo, Excluir, Limpar Concluídos, Agendador, Opções, Instalador & Extensão e Atualizar |
| **Indicador de atividade** | Quantidade de downloads ativos e velocidade total em tempo real |
| **Filtro** | Busca instantânea por nome do arquivo ou URL |
| **Card Solto / Minimizar para a Bandeja** | Alterna para o modo compacto ou esconde a janela, mantendo os downloads rodando |
| **Seletor de idioma (`🌐 PT / EN`)** | Alterna instantaneamente entre Português (PT-BR) e Inglês (EN-US) |
| **Lateral — Categorias** | Filtra a lista por categoria; o ícone de pasta abre a pasta da categoria |
| **Lateral — Status** | Em andamento e concluídos |
| **Lateral — Filas** | Fila Principal, Fila de Sincronização e filas customizadas, com botão **Iniciar** |
| **Lista de downloads** | Arquivo, tamanho, progresso, velocidade, tempo restante, categoria e data |
| **Rodapé** | Abrir pasta de downloads, total de itens e conexões ativas |

---

## Adicionando downloads

Há várias formas de entregar um link ao OpenIDM:

1. **Capturar Link** — clique no botão laranja na barra de ferramentas e cole a URL. Se houver uma URL na área de transferência, ela já vem preenchida.
2. **Navegador** — com a [extensão](EXTENSAO-NAVEGADOR.md) instalada, os downloads de tipos monitorados são capturados automaticamente.
3. **Menu de contexto** — clique com o botão direito em um link → **Baixar com o OpenIDM**.
4. **Área de transferência** — com o monitor ativo, ao copiar uma URL de download o OpenIDM pergunta se deseja baixá-la.
5. **Linha de comando** — `openidm.exe "https://exemplo.com/arquivo.zip"`.
6. **Bandeja** — botão direito no ícone → **Novo Download (Salvar Link)...**

Na tela de confirmação o OpenIDM consulta o servidor (*probe*) e mostra:

- **Nome do arquivo** (editável);
- **Categoria** detectada pela extensão e a **pasta de destino** correspondente;
- **Tamanho** e se o servidor aceita downloads segmentados (*Accept-Ranges*).

Você pode trocar a categoria, escolher outra pasta (incluindo pastas de download já usadas) e iniciar na hora ou apenas salvar na fila.

---

## Acompanhando um download

Dê um duplo clique em um item para abrir os **detalhes**:

- **Barra de segmentos** — cada trecho do arquivo e a conexão responsável por ele;
- **Velocidade, baixado e restante** em tempo real;
- **Log de conexões** — abertura, reuso de conexão (*keep-alive*), novas divisões, erros e reconexões;
- **Limite de velocidade** individual para aquele download.

### Status possíveis

| Status | Significado |
|---|---|
| **Na Fila** | Aguardando a vez na fila ou o horário agendado |
| **Conectando...** | Consultando o servidor (tamanho, suporte a `Range`, `ETag`) |
| **Baixando** | Em andamento |
| **Pausado** | Interrompido manualmente; pode ser retomado sem perder progresso |
| **Montando...** | Finalizando o arquivo parcial e movendo para o destino |
| **Concluído** | Arquivo final pronto na pasta de destino |
| **Parado** | Cancelado pelo usuário |
| **Erro** | Falhou após as tentativas configuradas — use **Retomar** para tentar de novo |

> Se o servidor não suportar downloads por partes, o OpenIDM baixa com **1 conexão** e indica o motivo nos detalhes.

---

## Card Solto

![Card Solto](images/card-solto.png)

O **Card Solto** é uma janela compacta pensada para ficar num canto da tela enquanto você trabalha:

- Barra de progresso, percentual, tamanho, velocidade e tempo restante de cada download;
- Botões de **pausar**, **parar**, **abrir pasta** e **remover**;
- Botão **Abrir** quando o download é concluído;
- Seletor de **conexões** (`8x`, `16x`, `28x`…) por download;
- **Pausar / Limpar / Novo** no topo e **App Completo** para voltar à janela principal.

A posição do card é lembrada entre as sessões. Ao abrir um link pela linha de comando ou pelo navegador com o app em segundo plano, a captura aparece diretamente no Card Solto.

---

## Bandeja do sistema

Fechar a janela **não encerra** o OpenIDM: ele continua baixando na bandeja (perto do relógio) e mostra uma notificação quando um download termina.

Menu do ícone (botão direito):

- 🚀 Abrir OpenIDM (App Completo)
- 🗂 Abrir Card Reduzido
- ➕ Novo Download (Salvar Link)...
- ⚡ Iniciar Fila Principal
- ⏸ Pausar Todos os Downloads
- ❌ Sair do OpenIDM

---

## Categorias

Cada download é classificado automaticamente pela extensão do arquivo e salvo na pasta da categoria:

| Categoria | Extensões padrão |
|---|---|
| 📦 Comprimidos | zip, rar, 7z, tar, gz, bz2, xz, tgz, zst, torrent |
| 📄 Documentos | pdf, doc, docx, xls, xlsx, ppt, pptx, txt, csv, epub, md |
| 🎵 Música | mp3, wav, flac, aac, ogg, m4a, wma, opus |
| 💻 Programas | exe, msi, iso, img, apk, dmg, deb, rpm, bat, ps1, safetensors, gguf, ckpt, pt, pth, onnx, bin, whl |
| 🎬 Vídeo | mp4, mkv, webm, avi, mov, flv, wmv, m4v, ts |
| 📁 Geral / Outros | qualquer outra extensão |

> Arquivos de modelos de IA (`.safetensors`, `.gguf`, `.onnx`…) já são reconhecidos — ideal para baixar modelos grandes do Hugging Face.

As pastas de cada categoria podem ser alteradas.

---

## Filas e agendador

Abra **Agendador** na barra de ferramentas.

| Opção | Descrição |
|---|---|
| **Downloads simultâneos** | Quantos itens da fila baixam ao mesmo tempo |
| **Horário de início (HH:MM)** | A fila começa sozinha nesse horário (em branco = manual) |
| **Horário de parada (HH:MM)** | Pausa todos os itens da fila nesse horário |
| **Cota de tráfego (MB)** | Para a fila ao atingir o volume definido |
| **Intervalo entre tentativas** | Tempo de espera antes de tentar de novo um item com erro (padrão 30 s) |
| **Máximo de tentativas** | `0` = infinito |
| **Executar ao concluir** | Programa ou comando executado quando a fila termina |
| **Desligar o computador** | Desliga o PC ao concluir a fila (pode ser cancelado) |
| **Fila de Sincronização** | Verifica `ETag`/`Last-Modified` e baixa de novo os arquivos que mudaram no servidor |

**Exemplo:** agende a Fila Principal para as **03:00**, limite a **2048 MB** e marque **Desligar o computador** — o OpenIDM baixa durante a madrugada e desliga o PC ao terminar.

---

## Opções

Em **Opções** você encontra as configurações avançadas:

### Conexão
- Máximo padrão de conexões por download (1 a 32, padrão 8);
- Tamanho mínimo para divisão de segmento (padrão 1024 KB);
- Limite global de velocidade (KB/s, `0` = ilimitado);
- Intervalo de gravação de checkpoints no SQLite.

### Proxy
- Modos: nenhum, proxy do sistema, HTTP/HTTPS, SOCKS5 ou arquivo PAC;
- Endereço, porta, usuário e senha opcionais.

### Captura
- Extensões capturadas automaticamente no navegador;
- Tecla modificadora para **ignorar** a captura (padrão `ALT`);
- Monitor da área de transferência.

### Exceções por site
Regras por host com curingas (`*.exemplo.com`): máximo de conexões, ignorar captura, limite e intervalo de tentativas. Útil para servidores que bloqueiam excesso de conexões.

### Credenciais por servidor
Usuário e senha (HTTP Basic), token (Bearer) ou FTP, aplicados automaticamente aos hosts correspondentes.

### Antivírus
Executável e parâmetros do scanner — use `[file]` no lugar do caminho do arquivo. Exemplo para o Microsoft Defender:

```text
Executável: C:\Program Files\Windows Defender\MpCmdRun.exe
Parâmetros: -Scan -ScanType 3 -File "[file]"
```

### Integração e Atualizações
- Instalação do Native Messaging Host e acesso ao assistente da extensão.
- **Botão `🔄 Atualizar` na Barra de Ferramentas**: verifica imediatamente no GitHub se há uma nova versão do OpenIDM. Quando um novo release sai no GitHub, o aplicativo também avisa automaticamente permitindo **Atualizar Agora** (baixa e instala sozinho) ou **Adiar por 24h**.

---

## Atalhos e dicas

- Segure **`ALT`** ao clicar em um link no navegador para baixar pelo próprio navegador.
- Use **Limpar Concluídos** para manter a lista enxuta (os arquivos não são apagados).
- Ao **Excluir**, você escolhe se o arquivo também deve ser removido do disco.
- Se um servidor bloquear muitas conexões, crie uma **exceção por site** limitando a 2–4 conexões.
- O OpenIDM roda como **instância única**: abrir o `.exe` novamente apenas traz a janela existente para frente.
