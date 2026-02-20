package database

import (
	"aibot/internal/models"
	"context"
	"fmt"
)

func (p *Postgres) GetUserByChatID(ctx context.Context, id int64) (models.User, error) {
	row := p.Pool.QueryRow(ctx, "SELECT telegram_id, user_name, created_at, statuses, subscription, subscription_status, subscription_end_at, subscription_start_at, limits_photo, limits_chat, remaining_photo, remaining_chat, used_photo, used_chat, used_trial FROM users WHERE telegram_id = $1", id)
	var user models.User
	err := row.Scan(&user.TelegramID, &user.UserName, &user.CreatedAt, &user.Statuses, &user.Subscription, &user.SubscriptionStatus, &user.SubscriptionEndAt, &user.SubscriptionStartAt, &user.LimitsPhoto, &user.LimitsChat, &user.RemainingPhoto, &user.RemainingChat, &user.UsedPhoto, &user.UsedChat, &user.UsedTrial)
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (p *Postgres) CreateUser(ctx context.Context, user models.User) error {
	_, err := p.Pool.Exec(ctx, `INSERT INTO users (telegram_id, user_name, created_at, statuses, subscription, subscription_status, subscription_end_at, subscription_start_at, limits_photo, limits_chat, remaining_photo, remaining_chat, used_photo, used_chat, used_trial) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (telegram_id) DO UPDATE SET
  user_name = EXCLUDED.user_name,
  statuses = EXCLUDED.statuses,
  subscription = EXCLUDED.subscription,
  subscription_status = EXCLUDED.subscription_status,
  subscription_end_at = EXCLUDED.subscription_end_at,
  subscription_start_at = EXCLUDED.subscription_start_at,
  limits_photo = EXCLUDED.limits_photo,
  limits_chat = EXCLUDED.limits_chat,
  remaining_photo = EXCLUDED.remaining_photo,
  remaining_chat = EXCLUDED.remaining_chat,
  used_photo = EXCLUDED.used_photo,
  used_chat = EXCLUDED.used_chat,
  used_trial = EXCLUDED.used_trial`,
		user.TelegramID, user.UserName, user.CreatedAt, user.Statuses, user.Subscription, user.SubscriptionStatus, user.SubscriptionEndAt, user.SubscriptionStartAt, user.LimitsPhoto, user.LimitsChat, user.RemainingPhoto, user.RemainingChat, user.UsedPhoto, user.UsedChat, user.UsedTrial)
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
}

func (p *Postgres) SetLimitsPhoto(ctx context.Context, chatID int64, limitsPhoto int) error {
	cmdTagLimits,err := p.Pool.Exec(ctx, "UPDATE users SET limits_photo = $1 WHERE telegram_id = $2", limits_photo, chatID)
	if err != nil {
		return err
	}

	if cmdTagLimits.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}

	сmdTagRemaining, err := p.Pool.Exec(ctx, "UPDATE users SET remeining_photo = $1 WHERE telegram_id = $2", limits_photo, chatID)
	if err != nil {
		return err
	}

	if cmdTagRemaining.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

func (p *Postgres) SetLimitsChat(ctx context.Context, chatID int64, limitsChat int) error {
	cmdTag, err := p.Pool.Exec(ctx, "UPDATE users SET limits_chat = $1 WHERE telegram_id = $2", limits_chat, chatID)
	if err != nil {
		return err
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

