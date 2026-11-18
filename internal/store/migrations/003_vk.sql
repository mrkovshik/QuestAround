-- VK-бот: какой пользователь VK каким игроком играет. У пользователя одна
-- текущая привязка — вступление в другую игру её заменяет.
CREATE TABLE vk_players (
    user_id    bigint PRIMARY KEY,
    run_id     text   NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    player_id  text   NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX vk_players_run ON vk_players (run_id);

-- До какого события журнала бот уже разослал уведомления по игре.
CREATE TABLE vk_cursors (
    run_id text   PRIMARY KEY REFERENCES runs (id) ON DELETE CASCADE,
    seq    bigint NOT NULL
);
