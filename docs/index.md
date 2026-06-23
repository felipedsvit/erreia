---
layout: home
title: Erreia — Kanban realtime em Go
description: Servidor Kanban minimalista com SSE + Postgres LISTEN/NOTIFY. Zero JS frameworks, binário único, Docker em um comando.
---

<div class="hero">
  <div class="container">
    <h1 class="hero-title">Erreia</h1>
    <p class="hero-subtitle">Kanban realtime em Go — SSE + Postgres LISTEN/NOTIFY, auth Argon2id, avatares MinIO. Sem React, sem Node.</p>
    <div class="hero-cta">
      <a href="{{ '/docs/quickstart/' | relative_url }}" class="btn btn-primary">Quickstart</a>
      <a href="https://github.com/felipedsvit/erreia" class="btn btn-secondary" target="_blank" rel="noopener">GitHub</a>
    </div>
  </div>
</div>

<div class="section">
  <div class="container">
    <h2 class="section-title">Features</h2>
    <div class="features-grid">
      <div class="feature-card">
        <h3>Realtime SSE</h3>
        <p>Push de eventos via Server-Sent Events. Hub central em memória distribui notificações do Postgres LISTEN/NOTIFY para todos os clientes conectados.</p>
      </div>
      <div class="feature-card">
        <h3>Auth Argon2id</h3>
        <p>Hashing de senhas com Argon2id (vencedor PHC 2015). Sessões em cookie httpOnly. Zero JWT.</p>
      </div>
      <div class="feature-card">
        <h3>Avatares MinIO</h3>
        <p>Upload de avatares para MinIO/S3 com validação de tipo e tamanho. Fallback para iniciais geradas no servidor.</p>
      </div>
      <div class="feature-card">
        <h3>Binário único ~15MB</h3>
        <p>Compilado com <code>CGO_ENABLED=0</code>. Imagem Docker <code>FROM scratch</code>. Deploy sem dependências de runtime.</p>
      </div>
      <div class="feature-card">
        <h3>Docker em 1 comando</h3>
        <p><code>docker compose up</code> sobe Postgres, MinIO e a aplicação. Sem configuração manual.</p>
      </div>
      <div class="feature-card">
        <h3>Zero JS frameworks</h3>
        <p>HTML + CSS + HTMX. O servidor processa toda a lógica. O cliente recebe fragmentos HTML prontos.</p>
      </div>
    </div>
  </div>
</div>

<div class="section">
  <div class="container">
    <h2 class="section-title">Quickstart</h2>

```bash
git clone https://github.com/felipedsvit/erreia
cd erreia
cp .env.example .env
docker compose up
```

Acesse `http://localhost:8080`. Crie uma conta, adicione um board e veja as atualizações em tempo real em outras abas.

  </div>
</div>

<div class="section">
  <div class="container">
    <h2 class="section-title">Stack</h2>

| Camada | Tecnologia |
|--------|-----------|
| Linguagem | Go 1.22+ |
| Banco de dados | PostgreSQL 16 |
| Realtime | SSE + LISTEN/NOTIFY |
| Auth | Argon2id (golang.org/x/crypto) |
| Storage | MinIO (compatível S3) |
| Frontend | HTMX + HTML/CSS |
| Deploy | Docker / Docker Compose |

  </div>
</div>

<div class="section">
  <div class="container">
    <p style="text-align:center">
      <a href="{{ '/docs/quickstart/' | relative_url }}" class="btn btn-primary">Ver documentação completa</a>
    </p>
  </div>
</div>
