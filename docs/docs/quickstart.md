---
layout: doc
title: Quickstart
description: Suba o Erreia localmente em menos de 5 minutos com Docker Compose.
---

O jeito mais rápido de rodar o Erreia é via Docker Compose. Você precisa de Docker e Docker Compose instalados.

## Pré-requisitos

- Docker 24+
- Docker Compose v2
- Git

## 1. Clone o repositório

```bash
git clone https://github.com/felipedsvit/erreia
cd erreia
```

## 2. Configure o ambiente

```bash
cp .env.example .env
```

O `.env.example` contém valores padrão que funcionam com o `docker-compose.yml` sem alterações. Para produção, troque as senhas e chaves.

## 3. Suba os serviços

```bash
docker compose up
```

Isso sobe três containers:
- **postgres** — banco de dados na porta 5432
- **minio** — object storage na porta 9000 (console: 9001)
- **app** — aplicação na porta 8080

Aguarde a mensagem `listening on :8080` no log.

## 4. Acesse

Abra `http://localhost:8080` no browser. Crie uma conta e experimente criar boards e cards.

Para ver realtime em ação: abra duas abas com o mesmo board e mova um card em uma delas — a outra atualiza sem reload.

## Rebuild da aplicação

O container `app` usa a imagem compilada. Para recompilar após mudanças no código:

```bash
docker compose build app
docker compose up app
```

Ou rode diretamente com Go (precisa de Postgres e MinIO rodando):

```bash
go run ./cmd/server
```

## Próximos passos

- [Configuração]({{ '/docs/configuracao/' | relative_url }}) — variáveis de ambiente detalhadas
- [Arquitetura]({{ '/docs/arquitetura/' | relative_url }}) — como os módulos internos se conectam
- [API]({{ '/docs/api/' | relative_url }}) — endpoints HTTP
