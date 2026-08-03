CREATE DATABASE IF NOT EXISTS vocab DEFAULT CHARACTER SET utf8mb4;
USE vocab;

CREATE TABLE IF NOT EXISTS words (
  id INT AUTO_INCREMENT PRIMARY KEY,
  word_key VARCHAR(255) NOT NULL,
  display_word VARCHAR(255) NOT NULL,
  translation TEXT,
  pos VARCHAR(100),
  review_count INT NOT NULL DEFAULT 1,
  first_added_at DATETIME NOT NULL,
  last_reviewed_at DATETIME NOT NULL,
  UNIQUE KEY uniq_word_key (word_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
