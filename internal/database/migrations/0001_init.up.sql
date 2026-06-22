-- Users
CREATE TABLE IF NOT EXISTS users (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email           text UNIQUE NOT NULL,
    password_hash   text NOT NULL,
    display_name    text NOT NULL,
    avatar_key      text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- Sessions (scs)
CREATE TABLE IF NOT EXISTS sessions (
    token   text PRIMARY KEY,
    data    bytea NOT NULL,
    expiry  timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions (expiry);

-- Boards
CREATE TABLE IF NOT EXISTS boards (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS boards_owner_idx ON boards (owner_id);

-- Columns (Kanban)
CREATE TABLE IF NOT EXISTS columns (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id   uuid NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    title      text NOT NULL,
    position   int  NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS columns_board_position_idx ON columns (board_id, position);

-- Cards
CREATE TABLE IF NOT EXISTS cards (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    column_id   uuid NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
    title       text NOT NULL,
    description text NOT NULL DEFAULT '',
    position    int  NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS cards_column_position_idx ON cards (column_id, position);
