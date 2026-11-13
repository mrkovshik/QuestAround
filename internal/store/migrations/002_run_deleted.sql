-- Корзина: удалённая игра скрыта и недоступна игрокам, через 30 дней стирается окончательно.
ALTER TABLE runs ADD COLUMN deleted_at timestamptz;
