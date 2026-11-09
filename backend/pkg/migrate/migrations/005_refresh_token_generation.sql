-- A rotation's successor is one generation after the token it replaces, and a
-- retry inside the grace window adds a sibling to that generation. Spending any
-- token retires every token of its generation or older, so siblings converge on
-- one chain at the next rotation.
ALTER TABLE refresh_tokens ADD COLUMN generation INTEGER NOT NULL DEFAULT 0;
