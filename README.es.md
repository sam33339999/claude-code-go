<p align="center">
  <img src="assets/logo.png" alt="Glaude Logo" width="200">
</p>

<h1 align="center">Glaude</h1>

<p align="center">
  <strong>🤖 Reimplementación en Go de Claude Code — Asistente de programación con IA en tu terminal</strong>
</p>

<p align="center">
  <a href="https://golang.org/dl/"><img src="https://img.shields.io/badge/go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Versión de Go"></a>
  <a href="https://goreportcard.com/report/github.com/tunsuy/glaude"><img src="https://goreportcard.com/badge/github.com/tunsuy/glaude?style=flat-square" alt="Go Report Card"></a>
  <a href="https://codecov.io/gh/tunsuy/glaude"><img src="https://codecov.io/gh/tunsuy/glaude/branch/main/graph/badge.svg?style=flat-square" alt="Cobertura"></a>
  <a href="https://pkg.go.dev/github.com/tunsuy/glaude"><img src="https://pkg.go.dev/badge/github.com/tunsuy/glaude.svg" alt="Go Reference"></a>
  <a href="https://github.com/tunsuy/glaude/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/tunsuy/glaude/ci.yml?branch=main&style=flat-square&logo=github&label=CI" alt="CI"></a>
  <a href="https://github.com/tunsuy/glaude/releases"><img src="https://img.shields.io/github/v/release/tunsuy/glaude?style=flat-square&logo=github" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="Licencia"></a>
  <a href="https://github.com/tunsuy/glaude/pulls"><img src="https://img.shields.io/badge/PRs-welcome-brightgreen?style=flat-square" alt="PRs Welcome"></a>
</p>

<p align="center">
  <a href="README.md">English</a> •
  <a href="README.zh-CN.md">简体中文</a> •
  <a href="README.zh-TW.md">繁體中文</a> •
  <a href="README.ja.md">日本語</a> •
  <a href="README.ko.md">한국어</a> •
  <a href="README.es.md">Español</a> •
  <a href="README.fr.md">Français</a>
</p>

---

<p align="center">
  <img src="assets/image.png" alt="Glaude - Sesión Interactiva" width="800">
  <br>
  <em>TUI interactiva con lectura de archivos y visualización de pensamiento en tiempo real</em>
</p>

<p align="center">
  <img src="assets/image1.png" alt="Glaude - Análisis de Proyecto" width="800">
  <br>
  <em>Análisis completo del proyecto con desglose de arquitectura</em>
</p>

---

## ¿Qué es esto?

Este proyecto es una **reimplementación completa en Go de [Claude Code](https://claude.ai/code)** — el CLI oficial de TypeScript de Anthropic — reescrito módulo por módulo en Go, cubriendo todas las características principales: TUI, uso de herramientas, sistema de permisos, coordinación multi-agente, protocolo MCP, gestión de sesiones y más.

> **🤖 Construido completamente por agentes de IA — cero código de producción escrito por humanos.** La construcción inicial (abril de 2026) fue producida por 9 agentes de IA Claude colaborando durante 9 días; desde agosto de 2026 una única sesión principal de Claude mantiene el proyecto contra un registro de brechas de paridad de comportamiento, con un responsable humano que revisa y fusiona. [Historia completa ↓](#construido-completamente-por-agentes-de-ia)

## ❤️ Patrocinadores

> [¿Quieres aparecer aquí?](https://github.com/tunsuy)

<table>
  <tr>
    <td width="180"><a href="https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075"><img src="assets/sponsors/orcarouter.png" alt="OrcaRouter" width="150"></a></td>
    <td><strong><a href="https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075">OrcaRouter</a></strong> — una puerta de enlace, todos los modelos. OrcaRouter enruta cada petición al modelo más adecuado entre más de 200 (de frontera y de código abierto), a precios de proveedor, con failover y registro de peticiones. Es compatible con OpenAI y se conecta directamente a Glaude — la configuración exacta está en la sección de <a href="#proveedores-de-api">Proveedores de API</a>.</td>
  </tr>
</table>

## Características

- **TUI interactiva** — Interfaz de terminal completa construida con [Bubble Tea](https://github.com/charmbracelet/bubbletea), con temas oscuro/claro
- **Uso de herramientas agénticas** — 35 herramientas integradas (archivo, shell, búsqueda, web, tareas, sub-agentes, …), todas mediadas a través de un pipeline de permisos de 9 capas
- **Code Mode** — El modelo escribe JavaScript/TypeScript que compone muchas llamadas a herramientas en UNA ejecución en sandbox (motor [moejs](https://github.com/MoeForge/moejs) en Go puro): replay, snippets reutilizables, auditoría de escrituras, aprobaciones por lotes — ver [Code Mode](#code-mode)
- **Subcomandos CLI** — `glaude mcp …`, `glaude plugin …`, `glaude auth …`, `glaude doctor`, `glaude update`; los grupos `mcp` y `plugin` producen una salida idéntica byte a byte a la del CLI de TypeScript
- **Gestión de plugins** — Instala/activa/desactiva plugins desde marketplaces locales, con validación completa de manifiestos (`glaude plugin validate`)
- **Coordinación multi-agente** — Genera sub-agentes en segundo plano para tareas paralelas
- **Soporte MCP** — Conecta herramientas externas a través del [Model Context Protocol](https://modelcontextprotocol.io) (transportes stdio + HTTP + WebSocket), importables desde Claude Desktop; las notificaciones push del servidor despiertan al agente por eventos (cero sondeo)
- **Memoria GLAUDE.md** — Carga automáticamente el contexto del proyecto desde archivos `GLAUDE.md` en el árbol de directorios
- **Gestión de sesiones** — Reanuda conversaciones anteriores; compacta automáticamente historiales largos
- **Modo Vim** — Atajos de teclado Vim opcionales en el área de entrada
- **Autenticación OAuth + clave API** — Inicia sesión con OAuth de Anthropic (`glaude auth login`) o proporciona una `ANTHROPIC_API_KEY`
- **Comandos slash integrados** — `/compact`, `/commit`, `/review`, `/model`, `/theme` y más (ver abajo)
- **Respuestas en streaming** — Streaming de tokens en tiempo real con visualización de bloques de pensamiento

## Arquitectura

Glaude está organizado en seis capas:

```
┌─────────────────────────────────────┐
│  CLI (cmd/glaude)                   │  punto de entrada Cobra
├─────────────────────────────────────┤
│  TUI (internal/tui)                 │  interfaz Bubble Tea MVU
├─────────────────────────────────────┤
│  Tools (internal/tools)             │  herramientas de archivo, shell, búsqueda, MCP
├─────────────────────────────────────┤
│  Core Engine (internal/engine)      │  streaming, despacho de herramientas, coordinador
├─────────────────────────────────────┤
│  Services (internal/api, oauth,     │  API de Anthropic, OAuth, cliente MCP
│            mcp, compact)            │
├─────────────────────────────────────┤
│  Infra (pkg/types, internal/config, │  tipos, configuración, estado, hooks, plugins
│         state, session, hooks)      │
└─────────────────────────────────────┘
```

Consulta [`docs/project/architecture.md`](docs/project/architecture.md) para un desglose detallado.

## Requisitos

- Go 1.26 o posterior
- Una [clave API de Anthropic](https://console.anthropic.com/) **o** cuenta Claude.ai (OAuth)

## Instalación

### Desde el código fuente

```bash
git clone https://github.com/tunsuy/glaude.git
cd glaude
make build
# El binario se coloca en ./bin/glaude
```

Añadir a tu `PATH`:

```bash
export PATH="$PATH:$(pwd)/bin"
```

### Usando `go install`

```bash
go install github.com/tunsuy/glaude/cmd/glaude@latest
```

## Inicio rápido

```bash
# Configura tu clave API (o usa OAuth — ver Autenticación abajo)
export ANTHROPIC_API_KEY=sk-ant-...

# Inicia una sesión interactiva en el directorio actual
claude

# Haz una pregunta única y sal
glaude -p "Explica el punto de entrada principal de este proyecto"

# Reanuda la sesión más reciente
glaude --resume
```

## Autenticación

**Clave API (recomendado para CI/scripts):**

```bash
export ANTHROPIC_API_KEY=sk-ant-...
# o guárdala de forma persistente:
glaude auth login --api-key sk-ant-...
```

**OAuth (recomendado para uso interactivo):**

```bash
glaude auth login    # abre el flujo OAuth en tu navegador
glaude auth status   # comprueba con qué cuenta has iniciado sesión
```

## Proveedores de API

Glaude soporta múltiples proveedores de API, permitiéndote usar no solo la API de Anthropic, sino también APIs compatibles con OpenAI.

### Proveedores soportados

| Proveedor | Descripción | Variables de entorno |
|-----------|-------------|---------------------|
| `direct` (predeterminado) | API directa de Anthropic | `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL` |
| `openai` | OpenAI y APIs compatibles | `OPENAI_API_KEY`, `OPENAI_BASE_URL` |
| `bedrock` | AWS Bedrock | Credenciales AWS vía entorno |
| `vertex` | Google Cloud Vertex AI | Credenciales GCP vía entorno |

### Usando APIs compatibles con OpenAI

Para usar OpenAI, DeepSeek, Qwen, Moonshot o cualquier API compatible con OpenAI:

**Método 1: Variables de entorno**

```bash
# Establecer proveedor a openai
export CLAUDE_PROVIDER=openai

# Establecer tu clave API
export OPENAI_API_KEY=sk-xxx

# Opcionalmente establecer una URL base personalizada (para servicios compatibles con OpenAI)
export OPENAI_BASE_URL=https://api.deepseek.com  # DeepSeek
# export OPENAI_BASE_URL=https://api.moonshot.cn/v1  # Moonshot
# export OPENAI_BASE_URL=http://localhost:11434/v1  # Ollama

# Establecer el modelo
export OPENAI_MODEL=deepseek-chat

# Ejecutar Claude Code
claude
```

**Método 2: Archivo de configuración**

Crea o edita `~/.config/claude-code/settings.json`:

```json
{
  "provider": "openai",
  "apiKey": "sk-xxx",
  "baseUrl": "https://api.openai.com",
  "model": "gpt-4-turbo",
  "openaiOrganization": "org-xxx",
  "openaiProject": "proj-xxx"
}
```

### Limitación de tasa

El control de tasa del lado del cliente se aplica uniformemente a todos los proveedores. Ambas variables son opcionales; sin definir＝sin límite (el reintento automático con backoff en 429 sigue activo).

| Variable de entorno | Descripción |
|---------------------|-------------|
| `CLAUDE_CODE_API_RPM` | Máximo de solicitudes por minuto (espaciado estricto entre inicios, p. ej. `60` = ≥1 s de intervalo) |
| `CLAUDE_CODE_API_MAX_CONCURRENT` | Máximo de solicitudes en curso, incluido streaming (los sub-agentes paralelos esperan si superan el límite) |

```bash
# Recomendado para proveedores con límite de tasa (p. ej. Qwen):
export CLAUDE_CODE_API_MAX_CONCURRENT=2   # limita ráfagas de sub-agentes paralelos
export CLAUDE_CODE_API_RPM=60             # opcional: espaciar solicitudes
```

Las solicitudes que aún reciben 429 se reintentan automáticamente con backoff exponencial; la cabecera `retry-after` del proveedor tiene prioridad.

### Notas específicas por proveedor

**OpenAI:**
- Soporta todos los modelos GPT-4 y GPT-3.5
- Soporte completo de herramientas/llamadas a funciones
- Respuestas en streaming

**DeepSeek:**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=sk-xxx
export OPENAI_BASE_URL=https://api.deepseek.com
export OPENAI_MODEL=deepseek-chat
```

**Ollama (local):**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_BASE_URL=http://localhost:11434/v1
export OPENAI_MODEL=llama3
```

**Azure OpenAI:**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=your-azure-key
export OPENAI_BASE_URL=https://your-resource.openai.azure.com
export OPENAI_MODEL=your-deployment-name
```

**OrcaRouter (puerta de enlace de IA):**
[OrcaRouter](https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075) es una puerta de enlace de IA que enruta cada petición al modelo más adecuado entre más de 200 (de frontera y de código abierto), a precios de proveedor, con failover y registro de peticiones. Es compatible con OpenAI, así que funciona directamente a través del proveedor `openai`:

```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=<your-orcarouter-key>
export OPENAI_BASE_URL=https://api.orcarouter.ai/v1
export OPENAI_MODEL=orcarouter/auto   # deja que OrcaRouter elija el modelo para cada petición
```

## Uso

### Modo interactivo

```
glaude [flags]
```

| Flag | Descripción |
|------|-------------|
| `--resume` | Reanuda la sesión más reciente |
| `--session <id>` | Reanuda una sesión específica por ID |
| `--model <name>` | Sobrescribe el modelo Claude predeterminado |
| `--dark` / `--light` | Fuerza tema oscuro o claro |
| `--vim` | Habilita atajos de teclado Vim |
| `-p, --print <prompt>` | No interactivo: ejecuta un solo prompt y sale |

### Subcomandos CLI

Comandos de gestión no interactivos. Los grupos `mcp` y `plugin` producen una salida idéntica byte a byte a la del CLI de TypeScript:

| Comando | Descripción |
|---------|-------------|
| `glaude mcp add <name> -- <command> [args…]` | Registra un servidor MCP por stdio (`--transport http <name> <url>` para remotos, `-e KEY=VAL` para variables de entorno) |
| `glaude mcp list` / `glaude mcp get <name>` | Lista los servidores configurados / comprueba el estado de uno |
| `glaude mcp add-json <name> <json>` | Registra un servidor a partir de JSON en bruto |
| `glaude mcp add-from-claude-desktop` | Importa servidores desde la configuración de Claude Desktop |
| `glaude mcp remove <name>` / `reset-project-choices` | Elimina un servidor / restablece las elecciones locales del proyecto |
| `glaude plugin install <plugin>` | Instala un plugin desde un marketplace configurado |
| `glaude plugin list` / `enable` / `disable` / `uninstall` / `update` | Gestiona los plugins instalados |
| `glaude plugin validate <path>` | Valida un manifiesto de plugin o de marketplace (`--strict`, `--json`) |
| `glaude plugin marketplace add/list/remove/update` | Gestiona marketplaces de plugins (rutas locales) |
| `glaude auth login` / `logout` / `status` | Autenticación OAuth o por clave API |
| `glaude doctor` | Comprobación del estado del entorno |

### Comandos slash

Escribe `/` en la entrada para ver todos los comandos disponibles:

| Comando | Descripción |
|---------|-------------|
| `/help` | Muestra todos los comandos |
| `/clear` | Limpia el historial de conversación |
| `/compact` | Resume el historial para reducir el uso de contexto |
| `/exit` | Sale de Claude Code |
| `/model` | Muestra o establece el modelo activo |
| `/theme` | Cambia el tema de colores |
| `/vim` | Alterna los atajos de teclado Vim |
| `/effort` | Establece el nivel de esfuerzo (low / medium / high) |
| `/status` | Muestra el estado de la sesión |
| `/cost` | Muestra el uso de tokens de esta sesión |
| `/session` | Muestra el ID de la sesión actual |
| `/memory` | Muestra los archivos de memoria GLAUDE.md cargados |
| `/dream` | Dispara manualmente la consolidación de memoria |
| `/review` | Revisa el diff actual de git con Claude |
| `/commit` | Pide a Claude que escriba y cree un commit de git |
| `/diff` | Muestra el diff actual de git |
| `/init` | Genera un GLAUDE.md para este proyecto |

`/config`, `/mcp`, `/resume` y `/terminal-setup` están completamente implementados (reanudación de sesiones, estado MCP, ajustes interactivos, configuración de teclas del terminal).

## Code Mode

En lugar de un viaje de ida y vuelta al LLM por cada herramienta, el modelo **escribe JavaScript/TypeScript que compone muchas llamadas a herramientas en UNA ejecución en sandbox** — la herramienta `CodeMode`, con el motor [moejs](https://github.com/MoeForge/moejs) en Go puro (sin CGO). Los datos intermedios permanecen en el sandbox; solo el valor de retorno y la salida de `console.log` vuelven al modelo (menos viajes, menos contexto).

```js
// Escrito por el modelo, ejecutado en el sandbox:
const src = await tools.Read({ file_path: "main.go" });
const todos = src.split("\n").filter(l => l.includes("TODO"));
console.log("found", todos.length);
return { count: todos.length, lines: todos };
```

### API del sandbox

| Dentro del sandbox | Qué hace |
|--------------------|----------|
| `tools.<Name>(input)` | Llama a cualquier herramienta registrada (asíncrono). `Promise.all` ejecuta llamadas en paralelo. **Cada llamada pasa por el pipeline completo de permisos.** |
| `codemode.search(query)` / `describe(name)` | Descubrimiento progresivo de herramientas (nombres por relevancia → esquema completo bajo demanda) |
| `codemode.history(limit)` | Ejecuciones recientes (id, código, resultado) para reutilizar |
| `codemode.replay(id)` | Re-ejecuta una ejecución registrada — los permisos siguen aplicándose |
| `codemode.run(name)` / `snippets()` | Ejecuta / lista snippets reutilizables con nombre |
| `codemode.audit(id)` | Efectos secundarios de escritura registrados en una ejecución |
| `codemode.approvals()` / `revoke(tool)` | Lista / revoca aprobaciones persistentes con ámbito CodeMode |
| `import { x } from "name"` | Módulos compartidos controlados desde un directorio gestionado por el host |
| `console.log(...)` | Salida capturada |

Campos de entrada: `code`, `typescript` (compilación con eliminación de tipos), `replay` (re-ejecutar por id de historial), `save_as` (guardar una ejecución exitosa como snippet con nombre), `batch_writes` (una aprobación para todas las escrituras).

### Modelo de seguridad

- **El sandbox no es autorización** — cada llamada interna ejecuta el mismo pipeline de permisos de 9 capas que una llamada directa. Sin acceso a sistema de archivos, red, env ni `eval`; la ejecución está limitada por timeout y límite de memoria.
- **Aprobación por lotes** — cuando una herramienta pregunta y tú apruebas, las llamadas posteriores a la *misma* herramienta dentro de esa ejecución quedan pre-aprobadas (solo ámbito de ejecución). Las concesiones por reglas se re-verifican en cada llamada, así que los cambios de modo en vivo (Shift+Tab) aplican de inmediato. Las denegaciones nunca se cachean.
- **Aprobaciones persistentes** — el diálogo de permisos ofrece «permitir siempre *tool* dentro de CodeMode»: una aprobación con ámbito CodeMode, otorgada por el usuario y revocable (`codemode.revoke`). Las reglas Deny siempre ganan.
- **Puerta de escritura** — con `batch_writes: true`, la primera llamada de escritura pregunta una vez (con una nota de ámbito de toda la ejecución); la decisión cubre todas las escrituras de esa ejecución. Las lecturas mantienen el flujo normal.
- **Spill de resultados** — las salidas de más de 32 KiB se escriben a un archivo; el modelo recibe una vista previa de 4 KiB más un puntero que puede `Read`/`Grep`.

### Funciones de flujo de trabajo

- **Replay**: re-ejecuta cualquier ejecución registrada por id (entrada `replay` o `codemode.replay`), con procedencia completa (`replay_of`) en el historial.
- **Snippets**: convierte ejecuciones exitosas en scripts reutilizables con nombre (`save_as` → `codemode.run`). Los snippets guardan el código completo; las ejecuciones anidadas comparten el ámbito de aprobación del llamador (profundidad de recursión limitada a 3).
- **Auditoría de escrituras**: cada llamada de escritura ejecutada queda registrada (herramienta + resumen de entrada) — un rastro honesto de *qué cambió*, consultable con `codemode.audit`.

Los datos de ejecución viven en `~/.glaude/codemode/`: `history.jsonl`, `spill/`, `modules/`, `snippets/`, `approvals.json`.

### Espera basada en eventos (cero sondeo)

Las notificaciones push de servidores MCP se enrutan a la cola de mensajes y despiertan al agente automáticamente — sin bucles de sondeo que quemen tokens. Combinado con Code Mode: un script hace el handshake/suscripción, el agente espera, y la notificación del servicio dispara la siguiente ejecución (que puede reutilizar lógica de snippets).

**Limitaciones conocidas**: sin rollback automático (el rastro de auditoría dice qué cambió; la restauración por git-checkpoint está en evaluación), replay usa el código registrado (≤2000 caracteres — guarda flujos largos como snippets), y la puerta de escritura aprueba sobre la primera escritura concreta más una nota de ámbito, no una vista previa completa del plan en dos fases.

Detalles de diseño: [`docs/project/design/codemode.md`](docs/project/design/codemode.md).

## Construido completamente por agentes de IA

> **Ningún humano escribió una sola línea de código de producción en este repositorio.**

Todo el proyecto — diseño de arquitectura, documentos de diseño detallados, implementación paralela, revisión de código, QA y pruebas de integración — fue producido por **9 agentes de IA Claude** colaborando en un flujo de trabajo multi-agente estructurado:

```
Agente PM          →  plan del proyecto, hitos, programación de tareas
Agente Tech Lead   →  diseño de arquitectura, revisión de documentos de diseño, revisión de código
Agent-Infra        →  capa de infraestructura (tipos, configuración, estado, sesión)
Agent-Services     →  capa de servicios (cliente API, OAuth, MCP, compactación)
Agent-Core         →  motor central (bucle LLM, despacho de herramientas, coordinador)
Agent-Tools        →  capa de herramientas (archivo, shell, búsqueda, web — 18 herramientas al momento de la construcción)
Agent-TUI          →  capa de UI (Bubble Tea MVU, temas, modo Vim)
Agent-CLI          →  capa de entrada (Cobra CLI, DI, fases de bootstrap)
Agente QA          →  estrategia de pruebas, aceptación por capa, pruebas de integración
```

Resultado: ~**7,000 líneas de código de producción + suite de pruebas completa en 9 días**, con `go test -race ./...` pasando.

Esta es una demostración real de que una aplicación Go no trivial y multicapa puede ser diseñada, implementada, revisada y entregada en su totalidad por agentes de IA que colaboran de forma asíncrona. Desde agosto de 2026 el proyecto se mantiene con el mismo espíritu — una única sesión principal de Claude trabajando sobre un registro de brechas de paridad de comportamiento frente al CLI original en TypeScript, con un responsable humano que revisa y fusiona. Sigue habiendo cero código de producción escrito por humanos (~33,000 líneas en la actualidad). El registro completo de decisiones vive en [`docs/project/`](docs/project/).

## Desarrollo

### Prerrequisitos

- Go 1.26+
- `golangci-lint` (opcional, para linting)

### Compilar y probar

```bash
# Compilar
make build

# Ejecutar todas las pruebas
make test

# Ejecutar pruebas con informe de cobertura
make test-cover

# Vet
make vet

# Lint (requiere golangci-lint)
make lint

# Compilar + probar + vet
make all
```

## Hoja de ruta

La construcción inicial (abril de 2026) entregó la arquitectura completa de seis capas en 9 días. Desde agosto de 2026 el proyecto está en **modo mantenimiento**: el desarrollo se guía por un registro de brechas de paridad de comportamiento ([`test/parity/cases.md`](test/parity/cases.md)) que compara este CLI con la versión original en TypeScript — cada brecha cerrada queda fijada byte a byte mediante pruebas, y cada divergencia deliberada se registra como `waived` con su motivo.

Hitos recientes:

- **2026-11-18** — Code Mode v1→v3 (composición de herramientas JS/TS en sandbox: aprobación por lotes, TypeScript, historial, spill, imports controlados, replay, snippets, auditoría de escrituras, aprobaciones persistentes, puerta de escritura); despertar MCP por eventos (notificaciones del servidor → agente, cero sondeo); kernel re-internalizado (sin dependencia de motor externo); migración a Bubble Tea v2; los 26 stubs visibles restantes rellenados (deuda: 0 stubs)
- **2026-09-05** — Los grupos CLI `mcp` (7 comandos) y `plugin` (8 comandos + el subárbol `marketplace`) quedaron integrados, con una salida idéntica byte a byte a la del CLI de TS; los stubs visibles para el usuario se redujeron de 42 a 27
- **2026-08-30** — Paridad de formato de `--version`; inventario completo de subcomandos frente al CLI de TS (§B11)
- **2026-08-29** — Proceso de modo mantenimiento implementado: líneas rojas de deuda aplicadas por CI, esqueleto de pruebas de paridad

El plan original por fases (v0.2.0 → v1.0.0) se conserva, con el estado de finalización de cada fase, en [`docs/ROADMAP.md`](docs/ROADMAP.md).

### Estado actual

```
✅ Hecho:       Motor central + streaming, TUI, 35 herramientas + sandbox Code Mode (v3),
                17 comandos slash, subcomandos CLI mcp/plugin/auth/doctor,
                pipeline de permisos de 9 capas, compactación de contexto
                (snip/micro/auto), OAuth, persistencia de sesiones,
                despertar MCP por eventos (cero sondeo)
🔧 Modo:        Mantenimiento — trabajo de paridad guiado por el registro de
                brechas frente al CLI de TypeScript
📉 Deuda:       11 TODO(dep) / 0 stubs visibles para el usuario, líneas rojas
                de solo-descenso aplicadas por CI
🚀 Lanzamiento: v0.8.0, binarios para 5 plataformas
```

## Contribuir

¡Las contribuciones son bienvenidas! Por favor lee [CONTRIBUTING.md](CONTRIBUTING.md) antes de enviar un pull request.

Lista de verificación rápida:
- Haz fork del repo y crea una rama de características
- Asegúrate de que `make test` y `make vet` pasen
- Escribe pruebas para la nueva funcionalidad
- Sigue el estilo de código existente (ejecuta `gofmt ./...`)
- Abre un pull request usando la plantilla proporcionada

## Seguridad

Para reportar una vulnerabilidad de seguridad, consulta [SECURITY.md](SECURITY.md). **No** abras un issue público de GitHub para errores de seguridad.

## Licencia

Este proyecto está licenciado bajo la Licencia MIT — consulta [LICENSE](LICENSE) para más detalles.

## Proyectos relacionados

- [claude-code](https://github.com/anthropics/claude-code) — el CLI original en TypeScript
- [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) — SDK oficial de Go para la API de Anthropic
- [Model Context Protocol](https://modelcontextprotocol.io) — estándar abierto para conectar IA a herramientas
