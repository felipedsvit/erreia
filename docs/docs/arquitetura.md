---
layout: doc
title: Arquitetura
description: Estrutura interna do Erreia — módulos, fluxo SSE e padrão de comunicação Postgres → Hub → Cliente.
---

O Erreia segue uma estrutura monolítica modular. Toda a lógica fica em um binário único; os módulos se comunicam por interfaces Go em vez de chamadas HTTP internas.

## Estrutura de módulos

```
erreia/
├── cmd/
│   └── server/        # entrypoint: inicializa deps e sobe HTTP
├── internal/
│   ├── auth/          # Argon2id hash, sessões cookie
│   ├── board/         # CRUD de boards e cards
│   ├── hub/           # SSE hub em memória
│   ├── listener/      # goroutine que ouve LISTEN/NOTIFY do Postgres
│   ├── storage/       # upload MinIO, validação de avatar
│   └── handler/       # handlers HTTP (chi router)
└── migrations/        # SQL com goose
```

## Fluxo de evento realtime

```
Browser A                  App Server              Postgres
   │                          │                       │
   │  POST /cards/:id/move    │                       │
   │─────────────────────────>│                       │
   │                          │  UPDATE cards         │
   │                          │──────────────────────>│
   │                          │                       │ NOTIFY board_events
   │                          │  <────────────────────│
   │                          │                       │
   │              Hub.Broadcast(boardID, payload)      │
   │                          │                       │
   │  SSE: event: card_moved  │                       │
   │<─────────────────────────│                       │
   │                          │                       │
Browser B (mesma tab)         │                       │
   │  SSE: event: card_moved  │                       │
   │<─────────────────────────│                       │
```

## Hub SSE

O `hub` mantém um mapa `boardID → []chan SSEEvent` em memória com mutex. Cada conexão SSE registra um canal no hub ao conectar e o remove ao desconectar.

```go
// internal/hub/hub.go
type Hub struct {
    mu      sync.RWMutex
    clients map[string][]chan SSEEvent
}

func (h *Hub) Subscribe(boardID string) (<-chan SSEEvent, func()) {
    ch := make(chan SSEEvent, 16)
    h.mu.Lock()
    h.clients[boardID] = append(h.clients[boardID], ch)
    h.mu.Unlock()
    return ch, func() { h.unsubscribe(boardID, ch) }
}
```

## Listener Postgres

Uma goroutine dedicada mantém uma conexão `pgconn` com `LISTEN board_events`. Quando chega uma notificação, decodifica o payload JSON e chama `Hub.Broadcast`.

```go
// internal/listener/listener.go
func (l *Listener) Listen(ctx context.Context) error {
    conn, _ := pgconn.Connect(ctx, l.dsn)
    conn.Exec(ctx, "LISTEN board_events").Close()
    for {
        n, _ := conn.WaitForNotification(ctx)
        var evt hub.SSEEvent
        json.Unmarshal([]byte(n.Payload), &evt)
        l.hub.Broadcast(evt.BoardID, evt)
    }
}
```

## Auth

Senhas são hasheadas com Argon2id (parâmetros: m=64MB, t=3, p=4). Sessões são armazenadas em cookies `httpOnly; SameSite=Lax; Secure`. Não há JWT.

## Storage

Avatares são enviados para MinIO via SDK Go. O handler valida Content-Type (`image/jpeg`, `image/png`, `image/webp`) e limita o tamanho a 2MB antes de fazer o upload.
