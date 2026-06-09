# RCSS TUI — Plano de Implementação (repositório standalone)

## Contexto

O RCSS original (https://github.com/dougmb/RCSS) é um conjunto de três scripts Bash (`uploadBackup.sh`, `cleanRemoteBackups.sh`, `restoreBackup.sh`) que envolvem o `rclone` para gerenciar backups por-projeto numa nuvem, configurados via `backup.env`.

Este projeto, **RCSS-tui**, nasce como **repositório novo e independente**: uma aplicação **Go pura** (Bubbletea) que reimplementa a lógica de backup chamando o binário `rclone` diretamente — sem scripts Bash e sem `backup.env`. A configuração vive num único `config.toml`. O objetivo é um app standalone, polido e publicável.

O repo original dos scripts permanece intacto em `origin` (GitHub) e serve apenas como **referência de portabilidade** da lógica.

## Decisões confirmadas

| Tema | Decisão | Implicação |
| --- | --- | --- |
| **Repositório** | Git novo nesta pasta (`RCSS-tui/`) | Histórico limpo. O `.git` antigo (que apontava p/ `dougmb/RCSS`) é reinicializado; o original segue salvo em `origin/RCSS`. |
| **Lógica de backup** | Go puro, sem scripts | upload/clean/restore reimplementados em Go, chamando o binário `rclone` via `exec.Command`. Sem dependência de bash; `rclone` continua sendo dependência de runtime (como sempre foi). |
| **Config** | Único `config.toml` em `~/.config/rcss/config.toml` | Sem `backup.env`, sem ponte/envgen. |
| **Tamanho mínimo** | 80×24 células | Abaixo disso, a TUI renderiza só o aviso centralizado `Not enough space to render panels`. |
| **Agendamento** | `crontab` do usuário | Bloco gerenciado e delimitado no crontab, sem root. O cron chamará o próprio binário da TUI em modo headless (ex: `rcss upload`). |
| **Idioma** | Inglês na UI e no código | Melhor para repositório público. |

## Stack

- `github.com/charmbracelet/bubbletea` — framework (arquitetura Elm)
- `github.com/charmbracelet/lipgloss` — estilos/tema
- `github.com/charmbracelet/bubbles` — `list`, `filepicker`, `viewport`, `progress`, `spinner`, `help`, `key`
- `github.com/charmbracelet/huh` — formulários (Settings, confirmações)
- `github.com/BurntSushi/toml` — config TOML
- Runtime: binário `rclone` instalado no PATH

## Layout do módulo (raiz do repo)

> O módulo Go vai na **raiz** do repositório (não mais em `tui/`). Module path: `github.com/dougmb/rcss-tui` (ajustar ao nome final do repo).

```
RCSS-tui/
├── go.mod / go.sum
├── main.go                entrypoint: subcomandos headless (upload/clean) p/ cron + modo TUI default
├── README.md              uso do app (reescrever — o atual descreve os scripts)
├── .gitignore             binário, ~/.config/rcss/ não aplica, mas ignora build artifacts e sync.log local
├── config/
│   └── config.go          struct Config + Load()/Save() do ~/.config/rcss/config.toml + defaults recomendados
├── rclone/
│   └── rclone.go          wrapper sobre o binário rclone: ListRemotes, Lsf(dirs/files), Copy(stream progresso), Delete
├── backup/
│   ├── upload.go          porta uploadBackup.sh (loop de projetos + invariantes de segurança)
│   ├── clean.go           porta cleanRemoteBackups.sh (safety-lock + dry-run/force)
│   ├── restore.go         porta restoreBackup.sh (listar projetos/arquivos + download)
│   └── log.go             logging para sync.log (níveis + bloco de SYNC SUMMARY)
├── cron/
│   └── cron.go            ler/escrever bloco "# RCSS-managed" no crontab do usuário
└── tui/
    ├── app.go             root model: window size, guard de tamanho, roteamento de telas, keybindings globais
    ├── styles.go          tema lipgloss
    ├── menu.go            menu principal (bubbles/list)
    ├── account.go         seleção de remote rclone + abrir `rclone config`
    ├── folder.go          seletor de pasta (bubbles/filepicker)
    ├── backups.go         lista backups no remoto + restore
    ├── upload.go          dispara backup.Upload com streaming + progress
    ├── clean.go           dry-run + execução de backup.Clean
    ├── settings.go        edição do config via huh
    ├── schedule.go        gerenciar cron
    └── logs.go            viewport sobre sync.log com destaque de ERROR/WARN
```

## Pacote `backup` — portar a lógica dos scripts para Go

A lógica de negócio sai dos `.sh` e vira código Go testável. **Preservar os invariantes de segurança** do RCSS original:

- **`Upload`** (de `uploadBackup.sh`): itera os subdiretórios de `BackupRoot`; pula dotfolders e os de `IgnoredFolders` (default `scripts config bin logs lost+found`); para cada "projeto" roda `rclone copy <proj> <remote>/<dest>/<proj>` com flags equivalentes (`--update --use-mmap --retries 3`, `-P` p/ progresso, `--exclude .*` se `SkipDotfiles`). **Limpeza local só no ramo de sucesso**: se `DeleteAfterUpload` remove todos os arquivos; senão remove os mais antigos que `RetentionDays`. Acumula contadores e escreve o bloco de SYNC SUMMARY no `sync.log`.
- **`Clean`** (de `cleanRemoteBackups.sh`): **safety-lock** — antes de deletar, confirma que existe backup recente no remoto (`rclone lsf --max-age <SafetyDays>d`); se não houver, aborta (a menos de `force`). Depois `rclone delete <remote>/<dest> --min-age <RemoteRetentionDays>d`, com suporte a `dry-run`.
- **`Restore`** (de `restoreBackup.sh`): lista projetos (`rclone lsf --dirs-only`) e arquivos (`--files-only`), e baixa o selecionado via `rclone copy` para `BackupRoot/<proj>/` ou destino escolhido.

As três funções recebem um callback/canal de linhas de progresso para alimentar a UI, e a mesma chamada serve ao modo headless (cron).

## Pacote `rclone` — wrapper do binário

Funções finas sobre `exec.Command("rclone", ...)`:
- `ListRemotes() []string` (`rclone listremotes`)
- `Lsf(path, dirsOnly|filesOnly) []string`
- `Copy(src, dst, opts, progress chan)` — lê stdout/stderr por linha
- `Delete(path, opts)`
- checagem de presença do binário no PATH no startup

## TUI

**Root model (`tui/app.go`)** em arquitetura Elm: guarda `width`/`height`, tela atual (enum), sub-models. `Update` trata `tea.WindowSizeMsg` e teclas globais (`q`/`ctrl+c` quit, `esc` volta, `?` ajuda). `View` aplica o guard de 80×24. Navegação via `switchScreenMsg`.

Funcionalidades → telas:
1. **Conta rclone** (`account.go`): `rclone.ListRemotes()`; "Configure new account" via `tea.ExecProcess(rclone config)` e re-lista ao voltar.
2. **Selecionar pasta** (`folder.go`): `bubbles/filepicker` somente-diretórios p/ `BackupRoot`/destino.
3. **Listar + restore** (`backups.go`): navega projetos→arquivos e dispara `backup.Restore` com progresso.
4. **Settings** (`settings.go`): form `huh` com defaults recomendados (retention 1d local / 15d remoto / safety 2d; delete-after-upload off; skip-dotfiles off). Salvar grava o TOML.
5. **Agendamento** (`schedule.go`): presets (upload diário HH:MM, clean semanal) → escreve bloco no crontab via `cron.go`.
6. **Logs** (`logs.go`): viewport sobre `sync.log`.
7. **Mínimo 80×24** e **navegação intuitiva**: guard + `bubbles/list` + `bubbles/help`.

## `config.toml`

Campos: `RemoteName`, `BackupRoot`, `DriveDestination`, `RetentionDays`, `RemoteRetentionDays`, `RemoteCleanupSafetyDays`, `DeleteAfterUpload`, `SkipDotfiles`, `IgnoredFolders`, `LogFile`. Carregado no startup; criado com defaults na primeira execução.

## Modo headless (para o cron)

`main.go` aceita subcomandos sem TUI: `rcss upload`, `rcss clean [--dry-run]`. O cron agendado pela tela de Schedule chama esses subcomandos, reusando exatamente o pacote `backup`. Sem subcomando → abre a TUI.

## Passos de implementação

1. **Setup do repo**: reinicializar git nesta pasta; mover o módulo Go p/ a raiz (module path `github.com/dougmb/rcss-tui`); `.gitignore`; remover scaffolding antigo `tui/models` `tui/runner`.
2. **config/**: struct, Load/Save TOML, defaults.
3. **rclone/**: wrapper + checagem de PATH.
4. **backup/**: portar Upload, Clean, Restore + log.go (com os invariantes de segurança).
5. **main.go**: roteamento TUI vs subcomandos headless.
6. **tui/app.go + styles.go**: root model, guard de tamanho, menu placeholder.
7. **menu.go** + roteamento.
8. **account.go** (ListRemotes + `rclone config`).
9. **folder.go** (filepicker).
10. **backups.go** (listar + restore com progresso).
11. **upload.go** e **clean.go** (streaming/progress; clean começa por dry-run).
12. **settings.go** (huh → salva TOML).
13. **cron/cron.go + schedule.go**.
14. **logs.go** (viewport).
15. **Polish**: help/keybindings, erros, loading, README do app, reescrever CLAUDE.md p/ o projeto Go.

## Verificação

- `go build ./...` e `go vet ./...` sem erros a cada etapa.
- TUI manual: terminal < 80×24 mostra o aviso; menu navega; `rclone config` abre/retorna sem corromper a tela; filepicker seleciona pasta; Settings salva o TOML; lista de backups popula do remoto (requer remote rclone configurado).
- Backup: comparar saída no viewport com `tail -f sync.log`; clean começa por dry-run; cleanup local só ocorre em upload bem-sucedido.
- Headless: `rcss upload` e `rcss clean --dry-run` funcionam sem TUI (são o que o cron chama).
- Cron: após agendar, `crontab -l` mostra o bloco `# >>> RCSS-managed >>>`; remover apaga só esse bloco.

## Notas

- **Invariantes de segurança** (do CLAUDE.md original) a preservar no port Go: cleanup local só no sucesso do upload; safety-lock do clean remoto (não deleta sem backup recente).
- Scripts `.sh` e `backup.env` atuais ficam como **referência** durante o port; removidos do repo novo após Upload/Clean/Restore estarem portados e verificados (continuam disponíveis em `origin/RCSS`).
- `CLAUDE.md` atual descreve o repo de scripts; será reescrito para o projeto Go ao final.
- `rclone` guarda credenciais no seu próprio config; `config.toml` não contém segredos de API mas contém paths/nome de remote — mantê-lo fora do versionamento (fica em `~/.config/rcss/`, naturalmente fora do repo).
