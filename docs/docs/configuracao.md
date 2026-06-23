---
layout: doc
title: Configuração
description: Variáveis de ambiente do Erreia com valores padrão e descrição.
---

O Erreia é configurado inteiramente por variáveis de ambiente. Copie `.env.example` para `.env` e ajuste os valores conforme seu ambiente.

## Banco de dados

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `DATABASE_URL` | `postgres://erreia:erreia@localhost:5432/erreia?sslmode=disable` | DSN completo do Postgres |
| `DB_MAX_OPEN_CONNS` | `25` | Conexões abertas máximas no pool |
| `DB_MAX_IDLE_CONNS` | `5` | Conexões ociosas máximas no pool |

## Aplicação

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `PORT` | `8080` | Porta HTTP do servidor |
| `SESSION_SECRET` | *(obrigatório)* | Chave para assinar cookies de sessão (mín. 32 bytes) |
| `BASE_URL` | `http://localhost:8080` | URL pública da aplicação (usada em links de e-mail) |
| `ENV` | `development` | `development` ou `production` (afeta logging e Secure cookie) |

## MinIO / S3

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `MINIO_ENDPOINT` | `localhost:9000` | Endpoint MinIO (sem `http://`) |
| `MINIO_ACCESS_KEY` | `minioadmin` | Access key |
| `MINIO_SECRET_KEY` | `minioadmin` | Secret key |
| `MINIO_BUCKET` | `erreia` | Nome do bucket para avatares |
| `MINIO_USE_SSL` | `false` | `true` em produção com TLS |

## Argon2id

Os parâmetros padrão são conservadores e seguros para produção. Ajuste apenas se tiver medições de latência.

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `ARGON2_MEMORY` | `65536` | Memória em KB (64 MB) |
| `ARGON2_ITERATIONS` | `3` | Número de iterações |
| `ARGON2_PARALLELISM` | `4` | Threads paralelas |

## Exemplo .env completo

```env
DATABASE_URL=postgres://erreia:s3cr3t@postgres:5432/erreia?sslmode=disable
SESSION_SECRET=troque-por-uma-string-aleatoria-de-32-bytes-aqui
BASE_URL=https://erreia.example.com
ENV=production

MINIO_ENDPOINT=minio:9000
MINIO_ACCESS_KEY=minio-access-key
MINIO_SECRET_KEY=minio-secret-key
MINIO_BUCKET=erreia
MINIO_USE_SSL=false

ARGON2_MEMORY=65536
ARGON2_ITERATIONS=3
ARGON2_PARALLELISM=4
```
