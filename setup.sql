CREATE TABLE todo (
  id         		SERIAL NOT null primary KEY,
  title      		TEXT NOT NULL,
  description		TEXT NOT NULL
);

INSERT INTO todo
  (title, description)
VALUES
  ('Get Groceries', 'Milk, eggs, coffee'),
  ('Finish this program', 'code code code'),
  ('Sleep', 'seriously, its late af');
