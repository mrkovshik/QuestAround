-- Организации (мультитенантность закладываем сразу).
CREATE TABLE organizations (
    id         text PRIMARY KEY,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO organizations (id, name) VALUES ('default', 'Default');

-- Версии сценариев: неизменяемы, игра ссылается на конкретную версию.
CREATE TABLE scenario_versions (
    id         text PRIMARY KEY,
    org_id     text NOT NULL REFERENCES organizations (id),
    slug       text NOT NULL,
    version    int  NOT NULL,
    source     text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, slug, version)
);

-- Проведения игр.
CREATE TABLE runs (
    id                  text PRIMARY KEY,
    org_id              text   NOT NULL REFERENCES organizations (id),
    scenario_version_id text   NOT NULL REFERENCES scenario_versions (id),
    title               text   NOT NULL DEFAULT '',
    public_code         text   NOT NULL UNIQUE,
    seed                bigint NOT NULL, -- uint64, хранится как те же 64 бита
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX runs_org_created ON runs (org_id, created_at DESC);

-- Журнал событий: только INSERT. Состояние игры — свёртка журнала.
CREATE TABLE events (
    run_id          text        NOT NULL REFERENCES runs (id),
    seq             bigint      NOT NULL,
    type            text        NOT NULL,
    at              timestamptz NOT NULL,
    team_id         text        NOT NULL DEFAULT '',
    player_id       text        NOT NULL DEFAULT '',
    client_event_id text        NOT NULL DEFAULT '',
    payload         jsonb       NOT NULL,
    PRIMARY KEY (run_id, seq)
);
