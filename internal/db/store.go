package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

type Player struct {
	ID        int
	Telegram  int64
	Username  string
	FullName  string
	Role      string
	Level     int
	Rating    int
	CreatedAt time.Time
}

type SystemConfig struct {
	RatingFormulaA       float64
	RatingFormulaB       float64
	DefaultCycleDuration int
	DefaultRatingTimeout int
}

type GameCycle struct {
	ID                   int
	CycleNumber          int
	StartTime            time.Time
	EndTime              time.Time
	DurationMinutes      int
	RatingTimeoutMinutes int
}

type RatingLimit struct {
	Level int
	Limit int
}

type RatingResult struct {
	RatingID     int64
	RatingChange int
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) EnsureSystemConfig(ctx context.Context) error {
	var exists bool
	if err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM system_config)").Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO system_config (rating_formula_a, rating_formula_b, default_cycle_duration_minutes, default_rating_timeout_minutes)
		VALUES ($1, $2, $3, $4)
	`, 1.0, 1.0, 60, 10)
	return err
}

func (s *Store) GetSystemConfig(ctx context.Context) (SystemConfig, error) {
	var cfg SystemConfig
	row := s.pool.QueryRow(ctx, `
		SELECT rating_formula_a, rating_formula_b, default_cycle_duration_minutes, default_rating_timeout_minutes
		FROM system_config
		ORDER BY id DESC
		LIMIT 1
	`)
	if err := row.Scan(&cfg.RatingFormulaA, &cfg.RatingFormulaB, &cfg.DefaultCycleDuration, &cfg.DefaultRatingTimeout); err != nil {
		return SystemConfig{}, err
	}
	return cfg, nil
}

func (s *Store) UpdateCycleDuration(ctx context.Context, minutes int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE system_config
		SET default_cycle_duration_minutes = $1, updated_at = NOW()
		WHERE id = (SELECT id FROM system_config ORDER BY id DESC LIMIT 1)
	`, minutes)
	return err
}

func (s *Store) UpdateRatingTimeout(ctx context.Context, minutes int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE system_config
		SET default_rating_timeout_minutes = $1, updated_at = NOW()
		WHERE id = (SELECT id FROM system_config ORDER BY id DESC LIMIT 1)
	`, minutes)
	return err
}

func (s *Store) UpsertRatingLimit(ctx context.Context, level int, limit int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO system_rating_limits (player_level, ratings_per_cycle)
		VALUES ($1, $2)
		ON CONFLICT (player_level) DO UPDATE
		SET ratings_per_cycle = EXCLUDED.ratings_per_cycle, updated_at = NOW()
	`, level, limit)
	return err
}

func (s *Store) CreatePlayer(ctx context.Context, telegramID int64, username, fullName string) (Player, error) {
	var player Player
	row := s.pool.QueryRow(ctx, `
		INSERT INTO players (telegram_id, username, full_name)
		VALUES ($1, $2, $3)
		RETURNING id, telegram_id, username, full_name, role, current_level, current_rating, created_at
	`, telegramID, username, fullName)
	if err := row.Scan(&player.ID, &player.Telegram, &player.Username, &player.FullName, &player.Role, &player.Level, &player.Rating, &player.CreatedAt); err != nil {
		return Player{}, err
	}
	return player, nil
}

func (s *Store) GetPlayerByTelegramID(ctx context.Context, telegramID int64) (Player, error) {
	var player Player
	row := s.pool.QueryRow(ctx, `
		SELECT id, telegram_id, username, full_name, role, current_level, current_rating, created_at
		FROM players
		WHERE telegram_id = $1
	`, telegramID)
	if err := row.Scan(&player.ID, &player.Telegram, &player.Username, &player.FullName, &player.Role, &player.Level, &player.Rating, &player.CreatedAt); err != nil {
		return Player{}, err
	}
	return player, nil
}

func (s *Store) GetPlayerByID(ctx context.Context, playerID int) (Player, error) {
	var player Player
	row := s.pool.QueryRow(ctx, `
		SELECT id, telegram_id, username, full_name, role, current_level, current_rating, created_at
		FROM players
		WHERE id = $1
	`, playerID)
	if err := row.Scan(&player.ID, &player.Telegram, &player.Username, &player.FullName, &player.Role, &player.Level, &player.Rating, &player.CreatedAt); err != nil {
		return Player{}, err
	}
	return player, nil
}

func (s *Store) GetPlayerByLinkHash(ctx context.Context, linkHash string) (Player, error) {
	var player Player
	row := s.pool.QueryRow(ctx, `
		SELECT p.id, p.telegram_id, p.username, p.full_name, p.role, p.current_level, p.current_rating, p.created_at
		FROM players p
		JOIN player_links pl ON pl.player_id = p.id
		WHERE pl.link_hash = $1
	`, linkHash)
	if err := row.Scan(&player.ID, &player.Telegram, &player.Username, &player.FullName, &player.Role, &player.Level, &player.Rating, &player.CreatedAt); err != nil {
		return Player{}, err
	}
	return player, nil
}

func (s *Store) UpdatePlayerProfile(ctx context.Context, telegramID int64, fullName, role string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE players
		SET full_name = $1, role = $2, updated_at = NOW()
		WHERE telegram_id = $3
	`, fullName, role, telegramID)
	return err
}

func (s *Store) SetPlayerRole(ctx context.Context, telegramID int64, role string) error {
	commandTag, err := s.pool.Exec(ctx, `
		UPDATE players
		SET role = $1, updated_at = NOW()
		WHERE telegram_id = $2
	`, role, telegramID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("player not found")
	}
	return nil
}

func (s *Store) CreatePlayerLink(ctx context.Context, playerID int) (string, error) {
	linkHash, err := generateHash(32)
	if err != nil {
		return "", err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO player_links (player_id, link_hash)
		VALUES ($1, $2)
		ON CONFLICT (player_id) DO UPDATE
		SET link_hash = EXCLUDED.link_hash, updated_at = NOW()
	`, playerID, linkHash)
	if err != nil {
		return "", err
	}
	return linkHash, nil
}

func (s *Store) GetPlayerLink(ctx context.Context, playerID int) (string, error) {
	var linkHash string
	row := s.pool.QueryRow(ctx, `
		SELECT link_hash FROM player_links WHERE player_id = $1
	`, playerID)
	if err := row.Scan(&linkHash); err != nil {
		return "", err
	}
	return linkHash, nil
}

func (s *Store) GetActiveCycle(ctx context.Context) (GameCycle, error) {
	var cycle GameCycle
	row := s.pool.QueryRow(ctx, `
		SELECT id, cycle_number, start_time, end_time, duration_minutes, rating_timeout_minutes
		FROM game_cycles
		WHERE is_active = TRUE
		ORDER BY start_time DESC
		LIMIT 1
	`)
	if err := row.Scan(&cycle.ID, &cycle.CycleNumber, &cycle.StartTime, &cycle.EndTime, &cycle.DurationMinutes, &cycle.RatingTimeoutMinutes); err != nil {
		return GameCycle{}, err
	}
	return cycle, nil
}

func (s *Store) EnsureActiveCycle(ctx context.Context, cfg SystemConfig) (GameCycle, error) {
	cycle, err := s.GetActiveCycle(ctx)
	if err == nil {
		if time.Now().Before(cycle.EndTime) {
			return cycle, nil
		}
		_, _ = s.pool.Exec(ctx, `
			UPDATE game_cycles SET is_active = FALSE, updated_at = NOW() WHERE id = $1
		`, cycle.ID)
	}

	var nextNumber int
	if err := s.pool.QueryRow(ctx, "SELECT COALESCE(MAX(cycle_number), 0) + 1 FROM game_cycles").Scan(&nextNumber); err != nil {
		return GameCycle{}, err
	}
	start := time.Now().UTC()
	end := start.Add(time.Duration(cfg.DefaultCycleDuration) * time.Minute)
	row := s.pool.QueryRow(ctx, `
		INSERT INTO game_cycles (cycle_number, start_time, end_time, duration_minutes, rating_timeout_minutes, is_active)
		VALUES ($1, $2, $3, $4, $5, TRUE)
		RETURNING id, cycle_number, start_time, end_time, duration_minutes, rating_timeout_minutes
	`, nextNumber, start, end, cfg.DefaultCycleDuration, cfg.DefaultRatingTimeout)
	var created GameCycle
	if err := row.Scan(&created.ID, &created.CycleNumber, &created.StartTime, &created.EndTime, &created.DurationMinutes, &created.RatingTimeoutMinutes); err != nil {
		return GameCycle{}, err
	}
	return created, nil
}

func (s *Store) GetRatingLimit(ctx context.Context, level int) (RatingLimit, error) {
	var limit RatingLimit
	row := s.pool.QueryRow(ctx, `
		SELECT player_level, ratings_per_cycle
		FROM system_rating_limits
		WHERE player_level = $1
	`, level)
	if err := row.Scan(&limit.Level, &limit.Limit); err != nil {
		return RatingLimit{}, err
	}
	return limit, nil
}

func (s *Store) CountRatingsByRaterInCycle(ctx context.Context, raterID, cycleID int) (int, error) {
	var count int
	row := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM player_ratings WHERE rater_id = $1 AND game_cycle_id = $2
	`, raterID, cycleID)
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) GetLastRatingBetween(ctx context.Context, raterID, ratedID int) (time.Time, error) {
	var created time.Time
	row := s.pool.QueryRow(ctx, `
		SELECT created_at FROM player_ratings
		WHERE rater_id = $1 AND rated_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, raterID, ratedID)
	if err := row.Scan(&created); err != nil {
		return time.Time{}, err
	}
	return created, nil
}

func (s *Store) CreateRating(ctx context.Context, rater Player, rated Player, cycle GameCycle, ratingType string, ratingChange int) (RatingResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RatingResult{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var ratingID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO player_ratings (rater_id, rated_id, rating_type, rating_value, base_value, game_cycle_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, rater.ID, rated.ID, ratingType, ratingChange, baseValue(ratingType), cycle.ID).Scan(&ratingID)
	if err != nil {
		return RatingResult{}, err
	}

	_, err = tx.Exec(ctx, `
		UPDATE players
		SET current_rating = current_rating + $1, updated_at = NOW()
		WHERE id = $2
	`, ratingChange, rated.ID)
	if err != nil {
		return RatingResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return RatingResult{}, err
	}
	return RatingResult{RatingID: ratingID, RatingChange: ratingChange}, nil
}

func (s *Store) CreateTransfer(ctx context.Context, sender Player, receiver Player, cycleID int, amount int, description string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	_, err = tx.Exec(ctx, `
		INSERT INTO rating_transfers (sender_id, receiver_id, amount, game_cycle_id, description)
		VALUES ($1, $2, $3, $4, $5)
	`, sender.ID, receiver.ID, amount, cycleID, description)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE players
		SET current_rating = current_rating - $1, updated_at = NOW()
		WHERE id = $2
	`, amount, sender.ID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE players
		SET current_rating = current_rating + $1, updated_at = NOW()
		WHERE id = $2
	`, amount, receiver.ID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Store) SetLevelBoundary(ctx context.Context, cycleID, level, minRating, maxRating int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO level_boundaries (game_cycle_id, level_number, min_rating, max_rating, target_percentage_min, target_percentage_max)
		VALUES ($1, $2, $3, $4, 0, 0)
		ON CONFLICT (game_cycle_id, level_number) DO UPDATE
		SET min_rating = EXCLUDED.min_rating, max_rating = EXCLUDED.max_rating, updated_at = NOW()
	`, cycleID, level, minRating, maxRating)
	return err
}

func (s *Store) GetLevelBoundaries(ctx context.Context, cycleID int) (map[int][2]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT level_number, min_rating, max_rating
		FROM level_boundaries
		WHERE game_cycle_id = $1
	`, cycleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	boundaries := make(map[int][2]int)
	for rows.Next() {
		var level, min, max int
		if err := rows.Scan(&level, &min, &max); err != nil {
			return nil, err
		}
		boundaries[level] = [2]int{min, max}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return boundaries, nil
}

func (s *Store) RecalculateLevels(ctx context.Context, cycleID int, boundaries map[int][2]int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	rows, err := tx.Query(ctx, `
		SELECT id, current_level, current_rating
		FROM players
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var playerID, currentLevel, currentRating int
		if err := rows.Scan(&playerID, &currentLevel, &currentRating); err != nil {
			return err
		}
		newLevel := currentLevel
		for level, bounds := range boundaries {
			if currentRating >= bounds[0] && currentRating <= bounds[1] {
				newLevel = level
				break
			}
		}
		if newLevel != currentLevel {
			if _, err := tx.Exec(ctx, `
				UPDATE players SET current_level = $1, updated_at = NOW() WHERE id = $2
			`, newLevel, playerID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO player_level_history (player_id, old_level, new_level, old_rating, new_rating, game_cycle_id)
				VALUES ($1, $2, $3, $4, $5, $6)
			`, playerID, currentLevel, newLevel, currentRating, currentRating, cycleID); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE game_cycles SET level_recalculation_done = TRUE, updated_at = NOW() WHERE id = $1
	`, cycleID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) LogOperation(ctx context.Context, operationType string, initiatorID *int, targetID *int, details json.RawMessage) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO operations_log (operation_type, initiator_id, target_id, details)
		VALUES ($1, $2, $3, $4)
	`, operationType, initiatorID, targetID, details)
	return err
}

func (s *Store) IsAdmin(ctx context.Context, telegramID int64) (bool, error) {
	var role string
	row := s.pool.QueryRow(ctx, `
		SELECT role FROM players WHERE telegram_id = $1
	`, telegramID)
	if err := row.Scan(&role); err != nil {
		return false, err
	}
	switch role {
	case "moderator", "admin", "super_admin":
		return true, nil
	default:
		return false, nil
	}
}

func generateHash(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate hash: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func baseValue(ratingType string) int {
	if ratingType == "like" {
		return 1
	}
	return -1
}
