CREATE DATABASE IF NOT EXISTS vocab DEFAULT CHARACTER SET utf8mb4;
USE vocab;

CREATE TABLE IF NOT EXISTS users (
  id INT AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(64) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  is_admin TINYINT(1) NOT NULL DEFAULT 0,
  disabled TINYINT(1) NOT NULL DEFAULT 0,
  daily_ai_limit_nano BIGINT NULL DEFAULT 5000000000,
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
  important_glosses JSON,
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

CREATE TABLE IF NOT EXISTS ai_daily_usage (
  user_id INT NOT NULL,
  usage_date DATE NOT NULL,
  spent_nano BIGINT NOT NULL DEFAULT 0,
  reserved_nano BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, usage_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ai_requests (
  request_id VARCHAR(32) PRIMARY KEY,
  parent_id VARCHAR(32) NOT NULL DEFAULT '',
  user_id INT NOT NULL,
  usage_date DATE NOT NULL,
  purpose VARCHAR(64) NOT NULL,
  provider VARCHAR(512) NOT NULL,
  model VARCHAR(128) NOT NULL,
  reserved_nano BIGINT NOT NULL,
  actual_nano BIGINT NULL,
  state VARCHAR(16) NOT NULL,
  details JSON NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_ai_user_day (user_id, usage_date, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
