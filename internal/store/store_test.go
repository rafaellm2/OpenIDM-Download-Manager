package store

import (
	"path/filepath"
	"testing"

	"openidm/internal/models"
)

// TestStoreSeedsAndCRUD verifica se categorias, filas, regras de site, logins e checkpoints são salvos corretamente.
func TestStoreSeedsAndCRUD(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store_test.sqlite")
	st, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("falha ao inicializar Store: %v", err)
	}
	defer st.Close()

	// 1. Verifica categorias padrão (Comprimidos, Documentos, Música, Programas, Vídeo, Geral).
	cats, err := st.ListCategories()
	if err != nil || len(cats) < 6 {
		t.Fatalf("esperado pelo menos 6 categorias padrão, obtido=%d (err=%v)", len(cats), err)
	}

	// 2. Verifica filas padrão (Fila Principal e Fila de Sincronização).
	queues, err := st.ListQueues()
	if err != nil || len(queues) < 2 {
		t.Fatalf("esperado pelo menos 2 filas padrão, obtido=%d (err=%v)", len(queues), err)
	}

	// 3. Testa persistência de SiteRule e SiteLogin.
	if err := st.SaveSiteRule(models.SiteRule{
		HostPattern:      "*.exemplo.org",
		MaxConnections:   2,
		IgnoreCapture:    true,
		RetryLimit:       5,
		RetryIntervalSec: 30,
	}); err != nil {
		t.Fatalf("erro SaveSiteRule: %v", err)
	}
	rules, _ := st.ListSiteRules()
	if len(rules) != 1 || rules[0].HostPattern != "*.exemplo.org" {
		t.Fatalf("SiteRule não persistida: %+v", rules)
	}

	if err := st.SaveSiteLogin(models.SiteLogin{
		HostPattern: "ftp.exemplo.org",
		Username:    "admin",
		Password:    "1234",
		AuthType:    "ftp",
	}); err != nil {
		t.Fatalf("erro SaveSiteLogin: %v", err)
	}
	logins, _ := st.ListSiteLogins()
	if len(logins) != 1 || logins[0].Username != "admin" {
		t.Fatalf("SiteLogin não persistido: %+v", logins)
	}
}

// TestStoreDownloadsPathAndMigration valida se caminhos antigos com OpenIDM são migrados para Downloads direto
func TestStoreDownloadsPathAndMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration_test.sqlite")
	st, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("falha ao inicializar Store: %v", err)
	}
	defer st.Close()

	// 1. Simula entrada antiga com subpasta OpenIDM na tabela categories
	legacyWinDir := `C:\Users\User\Downloads\OpenIDM\Documentos`
	_, err = st.db.Exec(`UPDATE categories SET default_dir = ? WHERE id = 'documents'`, legacyWinDir)
	if err != nil {
		t.Fatalf("erro ao inserir caminho legado: %v", err)
	}

	// 2. Simula entrada antiga com subpasta OpenIDM em app_settings
	legacySettingsJSON := `{"defaultSaveDir":"C:\\Users\\User\\Downloads\\OpenIDM","maxConnectionsPerDownload":4}`
	_, err = st.db.Exec(`UPDATE app_settings SET settings_json = ? WHERE id = 1`, legacySettingsJSON)
	if err != nil {
		t.Fatalf("erro ao atualizar settings legadas: %v", err)
	}

	// 3. Executa a migração (seedDefaults)
	if err := st.seedDefaults(); err != nil {
		t.Fatalf("erro ao executar seedDefaults: %v", err)
	}

	// 4. Verifica se a categoria foi migrada para C:\Users\User\Downloads\Documentos
	cats, err := st.ListCategories()
	if err != nil {
		t.Fatalf("erro ao listar categorias: %v", err)
	}
	for _, c := range cats {
		if c.ID == "documents" {
			expected := `C:\Users\User\Downloads\Documentos`
			if c.DefaultDir != expected {
				t.Errorf("caminho de categoria não foi migrado. Esperado: %s, obtido: %s", expected, c.DefaultDir)
			}
		}
	}

	// 5. Verifica se as configurações foram migradas
	settings, err := st.GetSettings()
	if err != nil {
		t.Fatalf("erro ao obter settings: %v", err)
	}
	expectedDir := `C:\Users\User\Downloads`
	if settings.DefaultSaveDir != expectedDir {
		t.Errorf("DefaultSaveDir não foi migrado. Esperado: %s, obtido: %s", expectedDir, settings.DefaultSaveDir)
	}

	// 6. Testa migração com barras no padrão Unix (/Downloads/OpenIDM/)
	legacyUnixDir := `/home/user/Downloads/OpenIDM/Musica`
	_, _ = st.db.Exec(`UPDATE categories SET default_dir = ? WHERE id = 'music'`, legacyUnixDir)
	legacyUnixSettings := `{"defaultSaveDir":"/home/user/Downloads/OpenIDM/","maxConnectionsPerDownload":4}`
	_, _ = st.db.Exec(`UPDATE app_settings SET settings_json = ? WHERE id = 1`, legacyUnixSettings)

	if err := st.seedDefaults(); err != nil {
		t.Fatalf("erro ao reexecutar seedDefaults: %v", err)
	}

	cats, _ = st.ListCategories()
	for _, c := range cats {
		if c.ID == "music" {
			expectedUnix := `/home/user/Downloads/Musica`
			if c.DefaultDir != expectedUnix {
				t.Errorf("caminho Unix não foi migrado. Esperado: %s, obtido: %s", expectedUnix, c.DefaultDir)
			}
		}
	}

	settings, _ = st.GetSettings()
	expectedUnixDir := `/home/user/Downloads/`
	if settings.DefaultSaveDir != expectedUnixDir {
		t.Errorf("DefaultSaveDir Unix não foi migrado. Esperado: %s, obtido: %s", expectedUnixDir, settings.DefaultSaveDir)
	}
}
