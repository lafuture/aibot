package database

import (
	"aibot/internal/models"
	"context"
	"fmt"
	"time"
)

const (
	TrialPhotoLimit   = 3
	TrialChatLimit    = 10
	LitePhotoLimit    = 15
	LiteChatLimit     = 1000
	PremiumPhotoLimit = 70
	PremiumChatLimit  = 1000
)

func (p *Postgres) GetUserByChatID(ctx context.Context, id int64) (models.User, error) {
	row := p.Pool.QueryRow(ctx, "SELECT telegram_id, user_name, created_at, statuses, subscription, subscription_end_at, subscription_start_at, limits_photo, limits_chat, remaining_photo, remaining_chat, used_photo, used_chat, used_trial FROM users WHERE telegram_id = $1", id)
	var user models.User
	err := row.Scan(&user.TelegramID, &user.UserName, &user.CreatedAt, &user.Statuses, &user.Subscription, &user.SubscriptionEndAt, &user.SubscriptionStartAt, &user.LimitsPhoto, &user.LimitsChat, &user.RemainingPhoto, &user.RemainingChat, &user.UsedPhoto, &user.UsedChat, &user.UsedTrial)
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (p *Postgres) CreateUser(ctx context.Context, user models.User) error {
	_, err := p.Pool.Exec(ctx, `INSERT INTO users (telegram_id, user_name, created_at, statuses, subscription, subscription_end_at, subscription_start_at, limits_photo, limits_chat, remaining_photo, remaining_chat, used_photo, used_chat, used_trial) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (telegram_id) DO UPDATE SET
  user_name = EXCLUDED.user_name`,
		user.TelegramID, user.UserName, user.CreatedAt, user.Statuses, user.Subscription, user.SubscriptionEndAt, user.SubscriptionStartAt, user.LimitsPhoto, user.LimitsChat, user.RemainingPhoto, user.RemainingChat, user.UsedPhoto, user.UsedChat, user.UsedTrial)
	return err
}

// EnsureUserExists создаёт запись в users, если её ещё нет (ON CONFLICT DO NOTHING).
// Подписка по умолчанию — пустая строка (подписка отсутствует).
func (p *Postgres) EnsureUserExists(ctx context.Context, chatID int64) error {
	_, err := p.Pool.Exec(ctx, `INSERT INTO users (telegram_id, user_name, created_at, statuses, subscription, subscription_end_at, subscription_start_at, limits_photo, limits_chat, remaining_photo, remaining_chat, used_photo, used_chat, used_trial)
VALUES ($1, '', NOW(), '', '', NULL, NULL, 0, 0, 0, 0, 0, 0, 0)
ON CONFLICT (telegram_id) DO NOTHING`,
		chatID)
	return err
}

func (p *Postgres) GetUsedTrial(ctx context.Context, chatID int64) (bool, error) {
	row := p.Pool.QueryRow(ctx, "SELECT used_trial FROM users WHERE telegram_id = $1", chatID)

	var trial int

	err := row.Scan(&trial)
	if err != nil {
		return false, err
	}

	if trial == 0 {
		return false, nil
	}

	return true, nil
}

func (p *Postgres) SetUsedTrial(ctx context.Context, chatID int64) error {
	cmdTag, err := p.Pool.Exec(ctx,
		"UPDATE users SET used_trial = 1 WHERE telegram_id = $1",
		chatID,
	)
	if err != nil {
		return err
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}

	if err := p.SetLimitsPhoto(ctx, chatID, TrialPhotoLimit); err != nil {
		return fmt.Errorf("set trial photo limits: %w", err)
	}
	if err := p.SetLimitsChat(ctx, chatID, TrialChatLimit); err != nil {
		return fmt.Errorf("set trial chat limits: %w", err)
	}

	return nil
}

func (p *Postgres) AddUsedPhoto(ctx context.Context, chatID int64) (bool, error) {
	cmdTag, err := p.Pool.Exec(ctx, "UPDATE users SET used_photo = used_photo + 1 WHERE telegram_id = $1", chatID)
	if err != nil {
		return false, err
	}

	if cmdTag.RowsAffected() == 0 {
		return false, fmt.Errorf("user not found")
	}

	cmdTag, err = p.Pool.Exec(ctx, "UPDATE users SET remaining_photo = remaining_photo - 1 WHERE telegram_id = $1", chatID)
	if err != nil {
		return false, err
	}

	if cmdTag.RowsAffected() == 0 {
		return false, fmt.Errorf("user not found")
	}

	return true, nil
}

func (p *Postgres) AddUsedChat(ctx context.Context, chatID int64) (bool, error) {
	cmdTag, err := p.Pool.Exec(ctx, "UPDATE users SET used_chat = used_chat + 1 WHERE telegram_id = $1", chatID)
	if err != nil {
		return false, err
	}

	if cmdTag.RowsAffected() == 0 {
		return false, fmt.Errorf("user not found")
	}

	cmdTag, err = p.Pool.Exec(ctx, "UPDATE users SET remaining_chat = remaining_chat - 1 WHERE telegram_id = $1", chatID)
	if err != nil {
		return false, err
	}

	if cmdTag.RowsAffected() == 0 {
		return false, fmt.Errorf("user not found")
	}

	return true, nil
}

func (p *Postgres) GetRemainingPhoto(ctx context.Context, chatID int64) (int, error) {
	row := p.Pool.QueryRow(ctx, "SELECT remaining_photo FROM users WHERE telegram_id = $1", chatID)

	var remainingPhoto int

	err := row.Scan(&remainingPhoto)
	if err != nil {
		return 0, err
	}

	return remainingPhoto, nil
}

func (p *Postgres) GetRemainingChat(ctx context.Context, chatID int64) (int, error) {
	row := p.Pool.QueryRow(ctx, "SELECT remaining_chat FROM users WHERE telegram_id = $1", chatID)

	var remainingChat int

	err := row.Scan(&remainingChat)
	if err != nil {
		return 0, err
	}

	return remainingChat, nil
}

func (p *Postgres) SetLimitsPhoto(ctx context.Context, chatID int64, limitsPhoto int) error {
	cmdTagLimits, err := p.Pool.Exec(ctx, "UPDATE users SET limits_photo = $1, remaining_photo = $2 WHERE telegram_id = $3", limitsPhoto, limitsPhoto, chatID)
	if err != nil {
		return err
	}

	if cmdTagLimits.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

func (p *Postgres) SetLimitsChat(ctx context.Context, chatID int64, limitsChat int) error {
	cmdTag, err := p.Pool.Exec(ctx, "UPDATE users SET limits_chat = $1, remaining_chat = $2 WHERE telegram_id = $3", limitsChat, limitsChat, chatID)
	if err != nil {
		return err
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

func (p *Postgres) SetSubscription(ctx context.Context, chatID int64, SubName string) error {
	timeNow := time.Now()

	var subscriptionEnd time.Time

	switch SubName {
	case "lite":
		subscriptionEnd = timeNow.Add(7 * 24 * time.Hour)
	case "premium":
		subscriptionEnd = timeNow.Add(30 * 24 * time.Hour)
	}

	cmdTag, err := p.Pool.Exec(ctx, "UPDATE users SET subscription = $1, subscription_start_at = $2, subscription_end_at = $3 WHERE telegram_id = $4", SubName, timeNow, subscriptionEnd, chatID)
	if err != nil {
		return err
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}

	switch SubName {
	case "lite":
		p.SetLimitsChat(ctx, chatID, LiteChatLimit)
		p.SetLimitsPhoto(ctx, chatID, LitePhotoLimit)
	case "premium":
		p.SetLimitsChat(ctx, chatID, PremiumChatLimit)
		p.SetLimitsPhoto(ctx, chatID, PremiumPhotoLimit)
	}

	return nil
}

func (p *Postgres) GetSubscription(ctx context.Context, chatID int64) (string, error) {
	row := p.Pool.QueryRow(ctx, "SELECT subscription FROM users WHERE telegram_id = $1", chatID)

	var sub string
	err := row.Scan(&sub)
	if err != nil {
		return "", err
	}

	return sub, nil
}

func (p *Postgres) ClearSubscription(ctx context.Context, chatID int64) error {
	_, err := p.Pool.Exec(ctx, "UPDATE users SET subscription = $1, subscription_start_at = NULL, subscription_end_at = NULL, limits_photo = 0, limits_chat = 0, remaining_photo = 0, remaining_chat = 0, used_photo = 0, used_chat = 0 WHERE telegram_id = $2", "End", chatID)
	if err != nil {
		return err
	}
	return nil
}

func (p *Postgres) IsSubscriptionEnd(ctx context.Context, chatID int64) (bool, error) {
	row := p.Pool.QueryRow(ctx, "SELECT subscription_end_at FROM users WHERE telegram_id = $1", chatID)

	var endAt time.Time

	err := row.Scan(&endAt)
	if err != nil {
		return false, err
	}

	// подписка истекла, если дата окончания в прошлом
	return endAt.IsZero() || time.Now().After(endAt), nil
}
