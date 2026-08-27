CREATE DATABASE IF NOT EXISTS vocab DEFAULT CHARACTER SET utf8mb4;
USE vocab;

CREATE TABLE IF NOT EXISTS users (
  id INT AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(64) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  is_admin TINYINT(1) NOT NULL DEFAULT 0,
  disabled TINYINT(1) NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  last_login_at DATETIME NULL,
  UNIQUE KEY uniq_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS sessions (
  token VARCHAR(64) PRIMARY KEY,
  user_id INT NOT NULL,
  expires_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL,
  CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS settings (
  name VARCHAR(64) PRIMARY KEY,
  value TEXT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS word_dictionary (
  word_key VARCHAR(255) NOT NULL PRIMARY KEY,
  display_word VARCHAR(255) NOT NULL,
  senses JSON,
  occurrence_count INT NOT NULL DEFAULT 1,
  first_seen_at DATETIME NOT NULL,
  last_updated_at DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS words (
  id INT AUTO_INCREMENT PRIMARY KEY,
  user_id INT NOT NULL,
  word_key VARCHAR(255) NOT NULL,
  display_word VARCHAR(255) NOT NULL,
  senses JSON,
  translating TINYINT(1) NOT NULL DEFAULT 0,
  translation_started_at DATETIME NULL,
  archived TINYINT(1) NOT NULL DEFAULT 0,
  review_count INT NOT NULL DEFAULT 1,
  first_added_at DATETIME NOT NULL,
  last_reviewed_at DATETIME NOT NULL,
  due_at DATETIME NULL,
  interval_days INT NOT NULL DEFAULT 0,
  ease_factor DECIMAL(4,2) NOT NULL DEFAULT 2.50,
  UNIQUE KEY uniq_user_word (user_id, word_key),
  KEY idx_words_user_archived (user_id, archived),
  CONSTRAINT fk_words_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
