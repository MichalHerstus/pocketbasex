CREATE TABLE "users" (
  "id" TEXT PRIMARY KEY,
  "password" TEXT,
  "tokenKey" TEXT,
  "email" TEXT,
  "emailVisibility" INTEGER,
  "verified" INTEGER,
  "name" TEXT,
  "avatar" TEXT,
  "relation" TEXT,
  "created" TEXT,
  "updated" TEXT,
  "created" TEXT NOT NULL DEFAULT (datetime('now')),
  "updated" TEXT NOT NULL DEFAULT (datetime('now'))
);
