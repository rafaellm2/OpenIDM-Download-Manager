import React, { createContext, useCallback, useContext, useEffect, useState } from "react";
import { AppLanguage, Category, Queue } from "../types";

// Idioma ativo global em memória para utilitários puros (ex: format.ts)
let activeLanguageInMemory: AppLanguage = "pt-BR";

// Normaliza qualquer código de idioma para "pt-BR" ou "en-US".
export function normalizeLanguage(lang?: string | null): AppLanguage {
  const clean = String(lang || "").trim().toLowerCase();
  if (clean.startsWith("en")) {
    return "en-US";
  }
  return "pt-BR";
}

// Retorna o idioma ativo atualmente na interface.
export function getActiveLanguage(): AppLanguage {
  return activeLanguageInMemory;
}

// Atualiza o idioma ativo em memória e sincroniza no localStorage e atributo lang do HTML.
export function setActiveLanguageInMemory(lang?: string | null): AppLanguage {
  const norm = normalizeLanguage(lang);
  activeLanguageInMemory = norm;
  try {
    localStorage.setItem("openidm_language", norm);
    if (typeof document !== "undefined" && document.documentElement) {
      document.documentElement.lang = norm;
    }
  } catch (_err) {
    // Ignora caso localStorage não esteja disponível
  }
  return norm;
}

// Inicializa a partir do localStorage se existir
try {
  const stored = localStorage.getItem("openidm_language");
  if (stored) {
    activeLanguageInMemory = normalizeLanguage(stored);
  }
} catch (_err) {
  // Ignora
}

const DEFAULT_CATEGORY_NAMES: Record<
  string,
  { "pt-BR": string; "en-US": string; aliases: string[] }
> = {
  compressed: {
    "pt-BR": "Comprimidos",
    "en-US": "Compressed",
    aliases: ["comprimidos", "compactados", "compressed"]
  },
  documents: {
    "pt-BR": "Documentos",
    "en-US": "Documents",
    aliases: ["documentos", "documents"]
  },
  music: {
    "pt-BR": "Música",
    "en-US": "Music",
    aliases: ["música", "musica", "músicas", "musicas", "music"]
  },
  programs: {
    "pt-BR": "Programas",
    "en-US": "Programs",
    aliases: ["programas", "programs"]
  },
  video: {
    "pt-BR": "Vídeo",
    "en-US": "Video",
    aliases: ["vídeo", "video", "vídeos", "videos"]
  },
  general: {
    "pt-BR": "Geral / Outros",
    "en-US": "General / Other",
    aliases: ["geral", "geral / outros", "general", "general / other"]
  }
};

const DEFAULT_QUEUE_NAMES: Record<
  string,
  { "pt-BR": string; "en-US": string; aliases: string[] }
> = {
  main: {
    "pt-BR": "Fila Principal de Downloads",
    "en-US": "Main Download Queue",
    aliases: [
      "fila principal",
      "fila principal de downloads",
      "main queue",
      "main download queue"
    ]
  },
  sync: {
    "pt-BR": "Fila de Sincronização",
    "en-US": "Sync Queue",
    aliases: ["fila de sincronização", "fila de sincronizacao", "sync queue"]
  }
};

// Traduz o nome de categorias padrão preservando nomes personalizados criados pelo usuário.
export function formatCategoryName(
  cat: Pick<Category, "id" | "name"> | undefined | null,
  lang: AppLanguage = activeLanguageInMemory
): string {
  if (!cat) {
    return DEFAULT_CATEGORY_NAMES.general[lang];
  }
  const entry = DEFAULT_CATEGORY_NAMES[cat.id];
  if (entry) {
    const trimmed = (cat.name || "").trim();
    const lower = trimmed.toLowerCase();
    if (
      !trimmed ||
      lower === cat.id.toLowerCase() ||
      entry.aliases.includes(lower)
    ) {
      return entry[lang];
    }
  }
  return cat.name;
}

// Traduz o nome de filas padrão preservando nomes de filas customizadas criadas pelo usuário.
export function formatQueueName(
  queue: Pick<Queue, "id" | "name"> | undefined | null,
  lang: AppLanguage = activeLanguageInMemory
): string {
  if (!queue) {
    return DEFAULT_QUEUE_NAMES.main[lang];
  }
  const entry = DEFAULT_QUEUE_NAMES[queue.id];
  if (entry) {
    const trimmed = (queue.name || "").trim();
    const lower = trimmed.toLowerCase();
    if (
      !trimmed ||
      lower === queue.id.toLowerCase() ||
      entry.aliases.includes(lower)
    ) {
      return entry[lang];
    }
  }
  return queue.name;
}

// Dicionário completo de traduções PT-BR e EN-US
export const TRANSLATIONS = {
  "pt-BR": {
    toolbar: {
      brandTooltip: "OpenIDM Desktop — Gerenciador de Downloads",
      captureLink: "Capturar Link",
      captureLinkTitle: "Abrir Card Solto para capturar e salvar download",
      resume: "Retomar",
      resumeTitle: "Retomar download selecionado",
      pause: "Pausar",
      pauseTitle: "Pausar download selecionado",
      stopAll: "Parar Tudo",
      stopAllTitle: "Parar todos os downloads ativos",
      delete: "Excluir",
      deleteTitle: "Excluir download selecionado da lista",
      clearCompleted: "Limpar Concluídos",
      clearCompletedTitle: "Limpar todos os downloads já concluídos da lista",
      scheduler: "Agendador",
      schedulerTitle: "Gerenciar Filas e Agendador",
      options: "Opções",
      optionsTitle: "Configurações gerais, idioma e conexões",
      installer: "Instalador & Extensão",
      installerTitle: "Instalação & Integração com o Navegador",
      speedTooltip: "Velocidade agregada em tempo real",
      activeCountLabel: (count: number, speed: string) => `${count} ativo(s) • ${speed}`,
      idleLabel: "Ocioso • 0 B/s",
      searchPlaceholder: "Filtrar por nome ou URL...",
      standaloneCardBtn: "🗂 Card Solto",
      standaloneCardTitle: "Minimizar o app principal no relógio e acompanhar downloads pelo Card Solto",
      minimizeToTray: "Minimizar para a Bandeja",
      minimizeToTrayTitle: "Minimizar janela para a bandeja do sistema",
      languageToggleTitle: "Alternar idioma: Português (PT-BR) / Inglês (EN-US)"
    },
    sidebar: {
      categoriesHeading: "Categorias",
      allDownloads: "🗂 Todos os Downloads",
      filterCategoryTitle: (name: string) => `Filtrar categoria ${name}`,
      openCategoryFolderTitle: (name: string) => `Abrir pasta de ${name} no Explorer`,
      statusHeading: "Status",
      inProgress: "⏳ Em Andamento",
      completed: "✅ Concluídos",
      queuesHeading: "Filas",
      queueRunningTitle: "Fila em execução (clique para pausar)",
      queueStoppedTitle: "Iniciar fila agora",
      queueActiveBadge: "Ativa",
      queueStartBtn: "Iniciar"
    },
    table: {
      dropOverlay: "Solte o link aqui para adicionar download",
      colFile: "Arquivo",
      colFileTitle: "Ordenar por nome do arquivo",
      colSize: "Tamanho",
      colSizeTitle: "Ordenar por tamanho total",
      colProgress: "Progresso",
      colProgressTitle: "Ordenar por progresso",
      colSpeed: "Velocidade",
      colSpeedTitle: "Ordenar por taxa de transferência",
      colRemaining: "Restante",
      colRemainingTitle: "Ordenar por tempo restante",
      colCategory: "Categoria",
      colAdded: "Adicionado",
      colAddedTitle: "Ordenar por data de adição",
      colActions: "Ações",
      resizeTitle: "Arrastar para redimensionar",
      emptyPrefix: "Nenhum download nesta visualização. Clique em ",
      emptyAction: "+ Capturar Link",
      emptySuffix: " ou arraste um link para cá.",
      unknownSize: "Desconhecido",
      downloadedSuffix: "baixados",
      pauseTitle: "Pausar download",
      startResumeTitle: "Iniciar / Retomar",
      openFileTitle: "Abrir arquivo concluído",
      openFolderTitle: "Abrir pasta no Explorer",
      detailsTitle: "Ver detalhes do download",
      deleteTitle: "Excluir download diretamente (para e remove)"
    },
    statusbar: {
      activeBackground: "Ativo em segundo plano",
      openDownloadsFolder: "📂 Abrir Pasta Downloads",
      openDownloadsFolderTitle: "Abrir pasta Downloads no Windows Explorer",
      summary: (total: number, conns: number) => `• Total: ${total} • Conexões: ${conns}x`
    },
    addModal: {
      title: "Salvar Novo Download",
      probing: "Verificando arquivo...",
      multiConn: " • Multi-conexão",
      checkLink: "Verifique o link",
      viewListBtn: (count: number) => `📋 Ver Lista (${count})`,
      viewListTitle: "Voltar para a lista compacta de downloads em andamento",
      fullAppBtn: "🗖 App Completo",
      fullAppTitle: "Abrir o Gerenciador de Downloads completo",
      closeTitle: "Fechar",
      capturedQueueLabel: (count: number) => `📥 ${count} links capturados:`,
      downloadAllBtn: (count: number) => `⚡ Baixar Todos (${count})`,
      downloadAllTitle: "Iniciar todos os downloads capturados",
      invalidUrlError: "Informe uma URL válida para download.",
      urlLabel: "URL do Download:",
      urlPlaceholder: "https://exemplo.com/arquivo.zip",
      pasteBtn: "📋 Colar",
      pasteTitle: "Colar link da área de transferência",
      fileNameLabel: "Nome do Arquivo:",
      fileNamePlaceholder: "Detectado automaticamente",
      categoryLabel: "Categoria:",
      autoSuffix: " (Auto)",
      saveDirLabel: "Salvar na Pasta:",
      folderBtn: "📁 Pasta...",
      folderBtnTitle: "Escolher pasta no Windows Explorer",
      cancelBtn: "Cancelar",
      downloadLaterBtn: "Baixar Depois",
      downloadLaterTitle: "Adicionar na fila e mostrar card compacto de downloads",
      downloadNowBtn: "⬇ Baixar Agora"
    },
    standaloneCard: {
      title: (count: number) => `Downloads (${count})`,
      speedTitle: "Velocidade total dos downloads ativos",
      activeStatus: (count: number, speed: string) =>
        `${count} ativo${count > 1 ? "s" : ""} • ${speed}`,
      idleStatus: "Fila parada • 0 B/s",
      savePendingBtn: (count: number) => `📥 Salvar (${count})`,
      savePendingTitle: "Confirmar links capturados aguardando na fila",
      pauseBtn: "⏸ Pausar",
      pauseTitle: "Pausar todos os downloads ativos",
      clearBtn: "🧹 Limpar",
      clearTitle: "Limpar itens concluídos deste card",
      newBtn: "＋ Novo",
      newTitle: "Adicionar novo link de download",
      fullAppBtn: "🗖 App Completo",
      fullAppTitle: "Abrir o Gerenciador de Downloads completo",
      closeTitle: "Ocultar card e manter ativo no relógio",
      emptyMessage: "Nenhum download em andamento no card.",
      addLinkBtn: "＋ Adicionar Link",
      openFullAppBtn: "🗖 Abrir App Completo"
    },
    cardList: {
      headerTitle: (count: number) => `Lista de Downloads (${count})`,
      downloadingCount: (count: number, speed: string) => `${count} baixando • ${speed}`,
      inListCount: (count: number) => `${count} na lista`,
      pauseAllBtn: "⏸ Pausar Todos",
      pauseAllTitle: "Pausar todos os downloads em andamento",
      clearBtn: "🧹 Limpar",
      clearTitle: "Limpar concluídos da visualização do card",
      pauseItemTitle: "Pausar download",
      resumeItemTitle: "Retomar / Iniciar download",
      stopItemTitle: "Parar download",
      openFileBtn: "📄 Abrir",
      openFileTitle: "Abrir arquivo baixado",
      openFolderTitle: "Abrir pasta no Windows Explorer",
      deleteItemTitle: "Excluir download diretamente (para e remove)",
      dismissItemTitle: "Ocultar item deste card (mantém no histórico do app)",
      completedBadge: "✓ Concluído",
      toggleSegmentsTitle: "Mostrar/ocultar fatias de conexão"
    },
    segmentBar: {
      sliceTitle: (idx: number, worker: number | string, start: string, end: string, pct: string, reused: boolean) =>
        `Segmento #${idx} | Worker #${worker} | [${start} - ${end}] (${pct}%)${reused ? " • Conexão Reutilizada" : ""}`,
      legendActiveStart: "Início de segmento ativo",
      legendDownloaded: "Baixado",
      legendReused: "Conexão Keep-Alive reutilizada",
      legendPending: "A baixar"
    },
    detailsModal: {
      savedIn: "Salvo em:",
      closeTitle: "Fechar (Esc)",
      downloadedLabel: "Baixado:",
      ofLabel: "de",
      unknownSize: "desconhecido",
      speedLabel: "Velocidade:",
      remainingLabel: "Restante:",
      hideConnections: "▴ Ocultar conexões",
      showConnections: "▾ Mostrar conexões",
      activeConnectionsCount: (count: number) => `${count} conexões ativas`,
      colConnection: "Conexão",
      colDownloaded: "Baixado",
      colSpeed: "Velocidade",
      colStatus: "Status",
      connectionNum: (id: number) => `Conexão #${id}`,
      segDownloading: "Baixando...",
      segCompleted: "Concluído",
      segWaiting: "Aguardando",
      openFolderBtn: "📂 Abrir Pasta",
      openFileBtn: "📄 Abrir Arquivo",
      deleteBtn: "🗑 Excluir",
      deleteTitle: "Excluir download e apagar arquivo do disco",
      pauseBtn: "⏸ Pausar",
      resumeBtn: "▶ Retomar Download",
      stopBtn: "⏹ Parar",
      closeBtn: "Fechar"
    },
    schedulerModal: {
      title: "Agendador e Gerenciador de Filas (Scheduler)",
      subtitle: "Configure horários de início/parada, fila de sincronização, quotas de tráfego e desligamento automático",
      registeredQueues: "FILAS CADASTRADAS",
      queueActive: "Ativa",
      queueStopped: "Parada",
      newCustomQueue: "NOVA FILA CUSTOMIZADA",
      newQueuePlaceholder: "Nome da nova fila...",
      syncQueueCheck: "Fila de Sincronização (ETag)",
      createQueueBtn: "+ Criar Fila",
      savedSuccessMsg: "Configurações da fila salvas com sucesso!",
      resetQuotaSuccessMsg: "Contador de tráfego da fila zerado.",
      syncQueueBannerTitle: "Fila de Sincronização:",
      syncQueueBannerText: "Verifica periodicamente o ETag / Last-Modified dos arquivos no servidor e re-baixa automaticamente caso tenham sido atualizados.",
      startTimeLabel: "Horário de Início Agendado (HH:MM):",
      startTimeHint: "Em branco = início manual",
      stopTimeLabel: "Horário de Parada Automática (HH:MM):",
      stopTimeHint: "Pausa todos os itens da fila neste horário",
      maxConcurrentLabel: "Downloads simultâneos na fila:",
      trafficQuotaLabel: "Quota / Limite de Tráfego da Fila (MB):",
      trafficQuotaPlaceholder: "0 = Sem limite (ex: 2048 para 2 GB)",
      resetQuotaBtn: (used: string) => `Zerar (${used})`,
      resetQuotaTitle: "Zerar tráfego acumulado",
      retryIntervalLabel: "Intervalo entre tentativas na fila (segundos):",
      retryIntervalHint: "Padrão OpenIDM: 30 segundos",
      maxRetriesLabel: "Máximo de tentativas por falha (0 = infinito):",
      runCommandLabel: "Executar programa/comando ao concluir a fila (Opcional):",
      runCommandPlaceholder: "Ex: C:\\Scripts\\pos-fila.bat",
      shutdownOnFinishLabel: "Desligar o computador ao terminar todos os downloads desta fila",
      abortShutdownBtn: "Abortar Desligamento do Windows",
      deleteQueueBtn: "🗑 Excluir Fila",
      stopQueueNowBtn: "🛑 Parar Fila Agora",
      startQueueNowBtn: "▶ Iniciar Fila Agora",
      saveChangesBtn: "💾 Salvar Alterações",
      closeBtn: "Fechar"
    },
    settingsModal: {
      title: "Opções do OpenIDM (Configurações Avançadas)",
      subtitle: "Idioma, Conexões, Exceções por Site, Captura MV3, Logins, Proxy SOCKS5/HTTP e Antivírus",
      tabConn: "Conexão e Exceções por Site",
      tabCapture: "Captura e Navegadores",
      tabLogins: (count: number) => `Logins de Sites (${count})`,
      tabProxy: "Proxy / SOCKS5",
      tabAv: "Pastas e Antivírus",
      savedSuccessMsg: "Configurações salvas com sucesso!",
      languageLabel: "Idioma da Interface (Language):",
      languagePt: "🇧🇷 Português (Brasil)",
      languageEn: "🇺🇸 English (United States)",
      connWarnBanner: "Nota OpenIDM: Número elevado de conexões acelera servidores com limite por socket, mas alguns servidores podem bloquear o IP se exceder o limite permitido por host.",
      maxConnLabel: "Máx. padrão de conexões por download:",
      connectionsUnit: (n: number) => `${n} conexões`,
      minSplitLabel: "Tamanho mínimo para divisão (MIN_SPLIT KB):",
      globalSpeedLabel: "Limite Global de Velocidade (KB/s, 0 = ilimitado):",
      siteExceptionsTitle: "Exceções por Site (Suporta Wildcards ex: *.exemplo.com)",
      siteHostPlaceholder: "Host ou Wildcard (ex: *.servidor.com)",
      siteMaxConnTitle: "Máximo de conexões para este host",
      siteIgnoreCheck: "Ignorar captura neste site",
      addRuleBtn: "+ Adicionar Regra",
      colHostPattern: "Padrão de Host",
      colMaxConn: "Máx. Conexões",
      colCapture: "Captura",
      colAction: "Ação",
      ignoreBrowserDownloads: "Ignorar (Navegador baixa)",
      captureActive: "Capturar",
      startWithWindowsCheck: "Iniciar o OpenIDM automaticamente junto com o Windows (oculto no canto do relógio)",
      monitorClipboardCheck: "Monitorar Área de Transferência (Clipboard) para URLs de arquivos",
      ignoreModifierLabel: "Tecla modificadora para IGNORAR captura no navegador:",
      altDefaultOption: "ALT (Padrão OpenIDM)",
      captureExtensionsLabel: "Capturar automaticamente downloads com as seguintes extensões:",
      nativeMessagingTitle: "Integração Native Messaging & Instalador da Extensão (Chrome / Edge / Firefox)",
      nativeMessagingDescPrefix: "A extensão MV3 está na pasta ",
      nativeMessagingDescSuffix: ". Você pode registrar o Native Host ou abrir o Instalador Oficial completo para configurar atalhos, ícone e ativar a extensão no navegador:",
      extIdPlaceholder: "ID da extensão Chrome/Edge (opcional)",
      registerNativeHostBtn: "Registrar Native Host",
      nativeHostRegisteredMsg: (path: string) => `Native Host registrado no Windows: ${path}`,
      openInstallerBtn: "📦 Abrir Instalador & Extensão",
      loginsTitle: "Credenciais Automáticas por Servidor (HTTP Basic, Bearer ou FTP)",
      loginHostPlaceholder: "Servidor (ex: *.meuservidor.com)",
      loginUserPlaceholder: "Usuário",
      loginPassPlaceholder: "Senha / Token",
      saveLoginBtn: "+ Salvar Login",
      colServer: "Servidor",
      colType: "Tipo",
      colUser: "Usuário",
      proxyModeLabel: "Modo de Proxy:",
      proxyNone: "Sem Proxy (Conexão Direta)",
      proxySystem: "Usar configurações de Proxy do Sistema",
      proxyHttp: "Proxy HTTP / HTTPS manual",
      proxySocks5: "Proxy SOCKS5",
      proxyPac: "Script de Configuração Automática (PAC)",
      proxyPacLabel: "URL ou Caminho do Arquivo PAC (.pac):",
      proxyPacPlaceholder: "http://empresa.local/proxy.pac ou C:\\proxy.pac",
      proxyHostLabel: "Endereço do Servidor Proxy:",
      proxyPortLabel: "Porta:",
      proxyUserLabel: "Usuário do Proxy (Opcional):",
      proxyPassLabel: "Senha do Proxy (Opcional):",
      defaultSaveDirLabel: "Pasta Padrão Geral para Downloads:",
      checkpointIntervalLabel: "Intervalo de gravação de Checkpoints no SQLite (segundos):",
      antivirusEnabledCheck: "Verificar arquivos automaticamente com Antivírus ao concluir download",
      antivirusPathLabel: "Executável do Antivírus:",
      antivirusArgsLabel: "Parâmetros da linha de comando (use [file] para o caminho do arquivo):",
      closeBtn: "Fechar",
      saveSettingsBtn: "💾 Salvar Configurações"
    },
    installerModal: {
      title: "Instalador Oficial — OpenIDM (v1.0.0)",
      subtitle: "Configura a inicialização com o Windows, atalhos de acesso rápido e integração com os navegadores",
      startingMsg: "Iniciando instalação do OpenIDM no Windows...",
      completedMsg: "Instalação concluída com sucesso!",
      failedMsg: "Falha ao executar instalador",
      launchedExtBanner: "✔ Navegador e pasta da extensão abertos! O caminho da extensão também foi copiado para a sua Área de Transferência (Ctrl+V).",
      openedExtFolderBanner: (dir: string) => `✔ Pasta aberta no Windows Explorer e caminho copiado para o Clipboard: ${dir}`,
      cardAutoStart: "Iniciar com o Windows",
      statusActive: "✔ Ativo",
      statusNotConfigured: "● Não configurado",
      cardBrowserIntegration: "Integração Navegador",
      statusIntegrated: "✔ Integrado",
      statusPending: "● Pendente",
      cardSystemShortcuts: "Atalhos do Sistema",
      statusConfigured: "✔ Configurados",
      cardDetectedBrowsers: "Navegadores Detectados",
      installDirLabel: "Diretório de Instalação do OpenIDM no Windows:",
      browseBtn: "📁 Procurar...",
      optionsHeading: "Opções de Instalação",
      optStartWithWinTitle: "Iniciar o OpenIDM automaticamente com o Windows",
      optStartWithWinSub: "Mantém o aplicativo ativo em segundo plano perto do relógio",
      optBrowserTitle: "Integrar ao navegador (Chrome, Edge, Brave e Firefox)",
      optBrowserSub: "Captura downloads automaticamente ao clicar em links no navegador",
      optShortcutsTitle: "Criar atalhos na Área de Trabalho e no Menu Iniciar",
      optShortcutsSub: "Facilita o acesso rápido ao aplicativo e ao desinstalador",
      optLaunchAfterTitle: "Abrir o OpenIDM após concluir a instalação",
      extActivationHeading: "🧩 Ativação da Extensão do Navegador",
      extFolderLabel: "Pasta da extensão:",
      openExtFolderBtn: "📂 Abrir Pasta da Extensão",
      installExtBtn: "🌐 Instalar Extensão no Navegador",
      installSuccessHeading: "✔ Instalação Concluída com Sucesso!",
      executableLabel: "Executável:",
      iconLabel: "Ícone:",
      closeBtn: "Fechar",
      installingBtn: "⏳ Instalando...",
      installBtn: "Instalar OpenIDM"
    }
  },
  "en-US": {
    toolbar: {
      brandTooltip: "OpenIDM Desktop — Multi-Segment Download Manager",
      captureLink: "Capture Link",
      captureLinkTitle: "Open Standalone Card to capture and save a download",
      resume: "Resume",
      resumeTitle: "Resume selected download",
      pause: "Pause",
      pauseTitle: "Pause selected download",
      stopAll: "Stop All",
      stopAllTitle: "Stop all active downloads",
      delete: "Delete",
      deleteTitle: "Delete selected download from list",
      clearCompleted: "Clear Completed",
      clearCompletedTitle: "Remove all completed downloads from the list",
      scheduler: "Scheduler",
      schedulerTitle: "Manage Queues and Scheduler",
      options: "Options",
      optionsTitle: "General settings, language and connections",
      installer: "Installer & Extension",
      installerTitle: "System Installation & Browser Integration",
      speedTooltip: "Real-time aggregate download speed",
      activeCountLabel: (count: number, speed: string) => `${count} active • ${speed}`,
      idleLabel: "Idle • 0 B/s",
      searchPlaceholder: "Filter by file name or URL...",
      standaloneCardBtn: "🗂 Compact Card",
      standaloneCardTitle: "Minimize main window to system tray and track downloads via Compact Card",
      minimizeToTray: "Minimize to Tray",
      minimizeToTrayTitle: "Minimize window to the Windows system tray",
      languageToggleTitle: "Switch language: English (EN-US) / Português (PT-BR)"
    },
    sidebar: {
      categoriesHeading: "Categories",
      allDownloads: "🗂 All Downloads",
      filterCategoryTitle: (name: string) => `Filter category ${name}`,
      openCategoryFolderTitle: (name: string) => `Open ${name} folder in Explorer`,
      statusHeading: "Status",
      inProgress: "⏳ In Progress",
      completed: "✅ Completed",
      queuesHeading: "Queues",
      queueRunningTitle: "Queue running (click to pause)",
      queueStoppedTitle: "Start queue now",
      queueActiveBadge: "Active",
      queueStartBtn: "Start"
    },
    table: {
      dropOverlay: "Drop link here to add download",
      colFile: "File",
      colFileTitle: "Sort by file name",
      colSize: "Size",
      colSizeTitle: "Sort by total size",
      colProgress: "Progress",
      colProgressTitle: "Sort by progress",
      colSpeed: "Speed",
      colSpeedTitle: "Sort by transfer rate",
      colRemaining: "Remaining",
      colRemainingTitle: "Sort by remaining time",
      colCategory: "Category",
      colAdded: "Added",
      colAddedTitle: "Sort by date added",
      colActions: "Actions",
      resizeTitle: "Drag to resize column",
      emptyPrefix: "No downloads in this view. Click ",
      emptyAction: "+ Capture Link",
      emptySuffix: " or drag a link here.",
      unknownSize: "Unknown",
      downloadedSuffix: "downloaded",
      pauseTitle: "Pause download",
      startResumeTitle: "Start / Resume",
      openFileTitle: "Open completed file",
      openFolderTitle: "Open folder in Explorer",
      detailsTitle: "View download details",
      deleteTitle: "Delete download directly (stops and removes)"
    },
    statusbar: {
      activeBackground: "Active in background",
      openDownloadsFolder: "📂 Open Downloads Folder",
      openDownloadsFolderTitle: "Open Downloads folder in Windows Explorer",
      summary: (total: number, conns: number) => `• Total: ${total} • Connections: ${conns}x`
    },
    addModal: {
      title: "Save New Download",
      probing: "Checking file...",
      multiConn: " • Multi-connection",
      checkLink: "Check link",
      viewListBtn: (count: number) => `📋 View List (${count})`,
      viewListTitle: "Return to compact list of active downloads",
      fullAppBtn: "🗖 Full App",
      fullAppTitle: "Open the full Download Manager window",
      closeTitle: "Close",
      capturedQueueLabel: (count: number) => `📥 ${count} captured links:`,
      downloadAllBtn: (count: number) => `⚡ Download All (${count})`,
      downloadAllTitle: "Start all captured downloads",
      invalidUrlError: "Please enter a valid download URL.",
      urlLabel: "Download URL:",
      urlPlaceholder: "https://example.com/file.zip",
      pasteBtn: "📋 Paste",
      pasteTitle: "Paste link from clipboard",
      fileNameLabel: "File Name:",
      fileNamePlaceholder: "Automatically detected",
      categoryLabel: "Category:",
      autoSuffix: " (Auto)",
      saveDirLabel: "Save in Folder:",
      folderBtn: "📁 Folder...",
      folderBtnTitle: "Choose folder in Windows Explorer",
      cancelBtn: "Cancel",
      downloadLaterBtn: "Download Later",
      downloadLaterTitle: "Add to queue and show compact downloads card",
      downloadNowBtn: "⬇ Download Now"
    },
    standaloneCard: {
      title: (count: number) => `Downloads (${count})`,
      speedTitle: "Total speed of active downloads",
      activeStatus: (count: number, speed: string) => `${count} active • ${speed}`,
      idleStatus: "Queue idle • 0 B/s",
      savePendingBtn: (count: number) => `📥 Save (${count})`,
      savePendingTitle: "Confirm captured links waiting in queue",
      pauseBtn: "⏸ Pause",
      pauseTitle: "Pause all active downloads",
      clearBtn: "🧹 Clear",
      clearTitle: "Clear completed items from this card",
      newBtn: "＋ New",
      newTitle: "Add new download link",
      fullAppBtn: "🗖 Full App",
      fullAppTitle: "Open the full Download Manager window",
      closeTitle: "Hide card and keep running in system tray",
      emptyMessage: "No active downloads in the card.",
      addLinkBtn: "＋ Add Link",
      openFullAppBtn: "🗖 Open Full App"
    },
    cardList: {
      headerTitle: (count: number) => `Download List (${count})`,
      downloadingCount: (count: number, speed: string) => `${count} downloading • ${speed}`,
      inListCount: (count: number) => `${count} in list`,
      pauseAllBtn: "⏸ Pause All",
      pauseAllTitle: "Pause all active downloads",
      clearBtn: "🧹 Clear",
      clearTitle: "Clear completed items from card view",
      pauseItemTitle: "Pause download",
      resumeItemTitle: "Resume / Start download",
      stopItemTitle: "Stop download",
      openFileBtn: "📄 Open",
      openFileTitle: "Open downloaded file",
      openFolderTitle: "Open folder in Windows Explorer",
      deleteItemTitle: "Delete download directly (stops and removes)",
      dismissItemTitle: "Hide item from this card (keeps in app history)",
      completedBadge: "✓ Completed",
      toggleSegmentsTitle: "Show/hide connection segments"
    },
    segmentBar: {
      sliceTitle: (idx: number, worker: number | string, start: string, end: string, pct: string, reused: boolean) =>
        `Segment #${idx} | Worker #${worker} | [${start} - ${end}] (${pct}%)${reused ? " • Reused Connection" : ""}`,
      legendActiveStart: "Active segment start",
      legendDownloaded: "Downloaded",
      legendReused: "Reused Keep-Alive connection",
      legendPending: "Remaining"
    },
    detailsModal: {
      savedIn: "Saved in:",
      closeTitle: "Close (Esc)",
      downloadedLabel: "Downloaded:",
      ofLabel: "of",
      unknownSize: "unknown",
      speedLabel: "Speed:",
      remainingLabel: "Remaining:",
      hideConnections: "▴ Hide connections",
      showConnections: "▾ Show connections",
      activeConnectionsCount: (count: number) => `${count} active connections`,
      colConnection: "Connection",
      colDownloaded: "Downloaded",
      colSpeed: "Speed",
      colStatus: "Status",
      connectionNum: (id: number) => `Connection #${id}`,
      segDownloading: "Downloading...",
      segCompleted: "Completed",
      segWaiting: "Waiting",
      openFolderBtn: "📂 Open Folder",
      openFileBtn: "📄 Open File",
      deleteBtn: "🗑 Delete",
      deleteTitle: "Delete download and remove file from disk",
      pauseBtn: "⏸ Pause",
      resumeBtn: "▶ Resume Download",
      stopBtn: "⏹ Stop",
      closeBtn: "Close"
    },
    schedulerModal: {
      title: "Queue Manager & Scheduler",
      subtitle: "Configure start/stop schedules, ETag sync queue, traffic quotas and automatic system shutdown",
      registeredQueues: "REGISTERED QUEUES",
      queueActive: "Active",
      queueStopped: "Stopped",
      newCustomQueue: "NEW CUSTOM QUEUE",
      newQueuePlaceholder: "New queue name...",
      syncQueueCheck: "Sync Queue (ETag)",
      createQueueBtn: "+ Create Queue",
      savedSuccessMsg: "Queue settings saved successfully!",
      resetQuotaSuccessMsg: "Queue traffic counter reset.",
      syncQueueBannerTitle: "Sync Queue:",
      syncQueueBannerText: "Periodically checks server ETag / Last-Modified headers and automatically re-downloads files when updated.",
      startTimeLabel: "Scheduled Start Time (HH:MM):",
      startTimeHint: "Blank = manual start",
      stopTimeLabel: "Automatic Stop Time (HH:MM):",
      stopTimeHint: "Pauses all items in the queue at this time",
      maxConcurrentLabel: "Concurrent downloads in queue:",
      trafficQuotaLabel: "Queue Traffic Quota / Limit (MB):",
      trafficQuotaPlaceholder: "0 = Unlimited (e.g. 2048 for 2 GB)",
      resetQuotaBtn: (used: string) => `Reset (${used})`,
      resetQuotaTitle: "Reset accumulated traffic",
      retryIntervalLabel: "Retry interval in queue (seconds):",
      retryIntervalHint: "OpenIDM Default: 30 seconds",
      maxRetriesLabel: "Max retries per failure (0 = infinite):",
      runCommandLabel: "Run program/command when queue finishes (Optional):",
      runCommandPlaceholder: "E.g. C:\\Scripts\\post-queue.bat",
      shutdownOnFinishLabel: "Shut down computer when all downloads in this queue finish",
      abortShutdownBtn: "Abort Windows Shutdown",
      deleteQueueBtn: "🗑 Delete Queue",
      stopQueueNowBtn: "🛑 Stop Queue Now",
      startQueueNowBtn: "▶ Start Queue Now",
      saveChangesBtn: "💾 Save Changes",
      closeBtn: "Close"
    },
    settingsModal: {
      title: "OpenIDM Options (Advanced Settings)",
      subtitle: "Language, Connections, Site Rules, MV3 Capture, Logins, SOCKS5/HTTP Proxy & Antivirus",
      tabConn: "Connections & Site Rules",
      tabCapture: "Capture & Browsers",
      tabLogins: (count: number) => `Site Logins (${count})`,
      tabProxy: "Proxy / SOCKS5",
      tabAv: "Folders & Antivirus",
      savedSuccessMsg: "Settings saved successfully!",
      languageLabel: "Interface Language (Idioma):",
      languagePt: "🇧🇷 Português (Brasil)",
      languageEn: "🇺🇸 English (United States)",
      connWarnBanner: "OpenIDM Note: Higher connection counts accelerate per-socket throttled servers, but some servers may block your IP if you exceed their per-host connection limit.",
      maxConnLabel: "Default max connections per download:",
      connectionsUnit: (n: number) => `${n} connections`,
      minSplitLabel: "Minimum split size (MIN_SPLIT KB):",
      globalSpeedLabel: "Global Speed Limit (KB/s, 0 = unlimited):",
      siteExceptionsTitle: "Site Exceptions & Rules (Supports Wildcards e.g. *.example.com)",
      siteHostPlaceholder: "Host or Wildcard (e.g. *.server.com)",
      siteMaxConnTitle: "Maximum connections for this host",
      siteIgnoreCheck: "Ignore capture on this site",
      addRuleBtn: "+ Add Rule",
      colHostPattern: "Host Pattern",
      colMaxConn: "Max Connections",
      colCapture: "Capture",
      colAction: "Action",
      ignoreBrowserDownloads: "Ignore (Browser downloads)",
      captureActive: "Capture",
      startWithWindowsCheck: "Launch OpenIDM automatically with Windows (hidden in system tray)",
      monitorClipboardCheck: "Monitor Clipboard for file download URLs",
      ignoreModifierLabel: "Modifier key to BYPASS capture in browser:",
      altDefaultOption: "ALT (OpenIDM Default)",
      captureExtensionsLabel: "Automatically capture downloads with the following extensions:",
      nativeMessagingTitle: "Native Messaging Integration & Extension Installer (Chrome / Edge / Firefox)",
      nativeMessagingDescPrefix: "The MV3 extension is located in ",
      nativeMessagingDescSuffix: ". You can register the Native Host or open the Official Installer to configure shortcuts, icon and activate the browser extension:",
      extIdPlaceholder: "Chrome/Edge Extension ID (optional)",
      registerNativeHostBtn: "Register Native Host",
      nativeHostRegisteredMsg: (path: string) => `Native Host registered on Windows: ${path}`,
      openInstallerBtn: "📦 Open Installer & Extension",
      loginsTitle: "Automatic Server Credentials (HTTP Basic, Bearer or FTP)",
      loginHostPlaceholder: "Server (e.g. *.myserver.com)",
      loginUserPlaceholder: "Username",
      loginPassPlaceholder: "Password / Token",
      saveLoginBtn: "+ Save Login",
      colServer: "Server",
      colType: "Type",
      colUser: "Username",
      proxyModeLabel: "Proxy Mode:",
      proxyNone: "No Proxy (Direct Connection)",
      proxySystem: "Use System Proxy Settings",
      proxyHttp: "Manual HTTP / HTTPS Proxy",
      proxySocks5: "SOCKS5 Proxy",
      proxyPac: "Automatic Configuration Script (PAC)",
      proxyPacLabel: "PAC File URL or Path (.pac):",
      proxyPacPlaceholder: "http://company.local/proxy.pac or C:\\proxy.pac",
      proxyHostLabel: "Proxy Server Address:",
      proxyPortLabel: "Port:",
      proxyUserLabel: "Proxy Username (Optional):",
      proxyPassLabel: "Proxy Password (Optional):",
      defaultSaveDirLabel: "Default General Downloads Folder:",
      checkpointIntervalLabel: "SQLite Checkpoint save interval (seconds):",
      antivirusEnabledCheck: "Automatically scan files with Antivirus when download completes",
      antivirusPathLabel: "Antivirus Executable:",
      antivirusArgsLabel: "Command line arguments (use [file] for the downloaded file path):",
      closeBtn: "Close",
      saveSettingsBtn: "💾 Save Settings"
    },
    installerModal: {
      title: "Official Installer — OpenIDM (v1.0.0)",
      subtitle: "Configures Windows startup, quick-access shortcuts and browser extension integration",
      startingMsg: "Starting OpenIDM installation on Windows...",
      completedMsg: "Installation completed successfully!",
      failedMsg: "Failed to run installer",
      launchedExtBanner: "✔ Browser and extension folder opened! The extension path was also copied to your Clipboard (Ctrl+V).",
      openedExtFolderBanner: (dir: string) => `✔ Folder opened in Windows Explorer and path copied to Clipboard: ${dir}`,
      cardAutoStart: "Start with Windows",
      statusActive: "✔ Active",
      statusNotConfigured: "● Not configured",
      cardBrowserIntegration: "Browser Integration",
      statusIntegrated: "✔ Integrated",
      statusPending: "● Pending",
      cardSystemShortcuts: "System Shortcuts",
      statusConfigured: "✔ Configured",
      cardDetectedBrowsers: "Detected Browsers",
      installDirLabel: "OpenIDM Installation Directory on Windows:",
      browseBtn: "📁 Browse...",
      optionsHeading: "Installation Options",
      optStartWithWinTitle: "Launch OpenIDM automatically with Windows",
      optStartWithWinSub: "Keeps the application active in the background in the system tray",
      optBrowserTitle: "Integrate with browsers (Chrome, Edge, Brave & Firefox)",
      optBrowserSub: "Automatically captures downloads when clicking links in your browser",
      optShortcutsTitle: "Create Desktop and Start Menu shortcuts",
      optShortcutsSub: "Provides quick access to the application and uninstaller",
      optLaunchAfterTitle: "Launch OpenIDM after installation finishes",
      extActivationHeading: "🧩 Browser Extension Activation",
      extFolderLabel: "Extension folder:",
      openExtFolderBtn: "📂 Open Extension Folder",
      installExtBtn: "🌐 Install Extension in Browser",
      installSuccessHeading: "✔ Installation Completed Successfully!",
      executableLabel: "Executable:",
      iconLabel: "Icon:",
      closeBtn: "Close",
      installingBtn: "⏳ Installing...",
      installBtn: "Install OpenIDM"
    }
  }
};

export type TranslationDictionary = typeof TRANSLATIONS["pt-BR"];

interface I18nContextValue {
  language: AppLanguage;
  t: TranslationDictionary;
  setLanguage: (lang: AppLanguage) => void;
  toggleLanguage: () => void;
}

const I18nContext = createContext<I18nContextValue>({
  language: activeLanguageInMemory,
  t: TRANSLATIONS[activeLanguageInMemory],
  setLanguage: () => {},
  toggleLanguage: () => {}
});

interface LanguageProviderProps {
  initialLanguage?: string;
  onLanguageChange?: (lang: AppLanguage) => void;
  children: React.ReactNode;
}

export const LanguageProvider: React.FC<LanguageProviderProps> = ({
  initialLanguage,
  onLanguageChange,
  children
}) => {
  const [language, setLanguageState] = useState<AppLanguage>(() =>
    setActiveLanguageInMemory(initialLanguage || activeLanguageInMemory)
  );

  useEffect(() => {
    if (initialLanguage) {
      const norm = setActiveLanguageInMemory(initialLanguage);
      setLanguageState(norm);
    }
  }, [initialLanguage]);

  const setLanguage = useCallback(
    (next: AppLanguage) => {
      const norm = setActiveLanguageInMemory(next);
      setLanguageState(norm);
      onLanguageChange?.(norm);
    },
    [onLanguageChange]
  );

  const toggleLanguage = useCallback(() => {
    const next: AppLanguage = language === "en-US" ? "pt-BR" : "en-US";
    setLanguage(next);
  }, [language, setLanguage]);

  const value: I18nContextValue = {
    language,
    t: TRANSLATIONS[language],
    setLanguage,
    toggleLanguage
  };

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
};

export function useI18n(): I18nContextValue {
  return useContext(I18nContext);
}
