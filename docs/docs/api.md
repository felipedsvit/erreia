---
layout: doc
title: API
description: Endpoints HTTP do Erreia — autenticação, boards, cards e SSE.
---

O Erreia expõe uma API HTTP convencional que retorna fragmentos HTML (HTMX) ou JSON dependendo do `Accept` header. Todos os endpoints que modificam dados requerem sessão autenticada.

## Autenticação

| Método | Path | Descrição |
|--------|------|-----------|
| `GET` | `/login` | Formulário de login |
| `POST` | `/login` | Autentica e cria sessão |
| `GET` | `/register` | Formulário de cadastro |
| `POST` | `/register` | Cria conta e cria sessão |
| `POST` | `/logout` | Destrói sessão |

## Boards

| Método | Path | Descrição |
|--------|------|-----------|
| `GET` | `/` | Lista boards do usuário |
| `POST` | `/boards` | Cria novo board |
| `GET` | `/boards/:id` | Exibe board com colunas e cards |
| `PUT` | `/boards/:id` | Atualiza nome do board |
| `DELETE` | `/boards/:id` | Apaga board e todos os cards |

## Cards

| Método | Path | Descrição |
|--------|------|-----------|
| `POST` | `/boards/:board_id/cards` | Cria card em uma coluna |
| `PUT` | `/cards/:id` | Atualiza título/descrição do card |
| `DELETE` | `/cards/:id` | Apaga card |
| `POST` | `/cards/:id/move` | Move card para outra coluna ou posição |

## SSE

| Método | Path | Descrição |
|--------|------|-----------|
| `GET` | `/boards/:id/events` | Stream SSE de eventos do board |

O endpoint SSE retorna `Content-Type: text/event-stream`. Cada evento tem o formato:

```
event: card_moved
data: {"card_id":42,"from_column":"todo","to_column":"doing","position":2}

event: card_created
data: {"card_id":43,"column":"todo","title":"Nova tarefa"}
```

O cliente HTMX conecta automaticamente ao abrir um board e escuta os eventos para atualizar a UI sem reload.

## Perfil

| Método | Path | Descrição |
|--------|------|-----------|
| `GET` | `/profile` | Página de perfil do usuário |
| `PUT` | `/profile` | Atualiza nome e e-mail |
| `POST` | `/profile/avatar` | Upload de avatar (multipart/form-data) |
