package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"rts_for_rating_on_larp/internal/db"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5"
	"github.com/skip2/go-qrcode"
)

const (
	rolePlayer     = "player"
	roleModerator  = "moderator"
	roleAdmin      = "admin"
	roleSuperAdmin = "super_admin"
)

type Bot struct {
	api         *tgbotapi.BotAPI
	store       *db.Store
	log         *slog.Logger
	botLinkBase string
}

func New(api *tgbotapi.BotAPI, store *db.Store, log *slog.Logger, botLinkBase string) *Bot {
	botLinkBase = strings.TrimSpace(botLinkBase)
	if botLinkBase == "" {
		botLinkBase = "https://t.me/novy_rim_bot"
	}
	return &Bot{api: api, store: store, log: log, botLinkBase: strings.TrimRight(botLinkBase, "/")}
}

func (b *Bot) WebhookHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			b.log.Info("webhook non-post request", "method", r.Method, "path", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			return
		}

		defer r.Body.Close()

		var update tgbotapi.Update
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			if errors.Is(err, io.EOF) {
				b.log.Info("empty webhook payload", "path", r.URL.Path)
				w.WriteHeader(http.StatusOK)
				return
			}
			b.log.Error("decode update", "error", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var (
			messageID int
			chatID    int64
			fromID    int64
			command   string
		)
		if update.Message != nil {
			messageID = update.Message.MessageID
			chatID = update.Message.Chat.ID
			if update.Message.From != nil {
				fromID = update.Message.From.ID
			}
			command = update.Message.Command()
		}
		if update.CallbackQuery != nil {
			if update.CallbackQuery.From != nil {
				fromID = update.CallbackQuery.From.ID
			}
			command = "callback"
		}
		b.log.Info("update received",
			"update_id", update.UpdateID,
			"message_id", messageID,
			"chat_id", chatID,
			"from_id", fromID,
			"command", command,
			"has_message", update.Message != nil,
			"has_callback", update.CallbackQuery != nil,
		)

		ctx := r.Context()
		switch {
		case update.Message != nil:
			if err := b.handleMessage(ctx, update.Message); err != nil {
				b.log.Error("handle message", "error", err)
			}
		case update.CallbackQuery != nil:
			if err := b.handleCallback(ctx, update.CallbackQuery); err != nil {
				b.log.Error("handle callback", "error", err)
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (b *Bot) handleMessage(ctx context.Context, message *tgbotapi.Message) error {
	if !message.IsCommand() {
		b.log.Info("non-command message ignored", "chat_id", message.Chat.ID, "message_id", message.MessageID)
		return nil
	}

	command := message.Command()
	var fromID int64
	if message.From != nil {
		fromID = message.From.ID
	}
	b.log.Info("command received", "command", command, "chat_id", message.Chat.ID, "from_id", fromID, "message_id", message.MessageID)

	var err error
	switch command {
	case "start":
		err = b.handleStart(ctx, message)
	case "register":
		err = b.handleRegister(ctx, message)
	case "my_link":
		err = b.handleMyLink(ctx, message)
	case "add_player":
		err = b.handleAddPlayer(ctx, message)
	case "set_cycle_duration":
		err = b.handleSetCycleDuration(ctx, message)
	case "set_rating_timeout":
		err = b.handleSetRatingTimeout(ctx, message)
	case "set_rating_limits":
		err = b.handleSetRatingLimits(ctx, message)
	case "set_level_boundary":
		err = b.handleSetLevelBoundary(ctx, message)
	case "apply_level_recalc":
		err = b.handleApplyLevelRecalc(ctx, message)
	case "create_admin":
		err = b.handleCreateAdmin(ctx, message)
	case "transfer":
		err = b.handleTransfer(ctx, message)
	default:
		err = b.reply(message.Chat.ID, "Неизвестная команда.")
	}
	if err != nil {
		b.log.Error("command failed", "command", command, "chat_id", message.Chat.ID, "from_id", fromID, "error", err)
		return err
	}
	b.log.Info("command handled", "command", command, "chat_id", message.Chat.ID, "from_id", fromID)
	return nil
}

func (b *Bot) handleCallback(ctx context.Context, callback *tgbotapi.CallbackQuery) error {
	parts := strings.Split(callback.Data, ":")
	if len(parts) < 2 {
		return b.answerCallback(callback.ID, "Некорректный запрос.")
	}

	actor, err := b.ensurePlayer(ctx, callback.From)
	if err != nil {
		return b.answerCallback(callback.ID, "Не удалось определить игрока.")
	}

	action := parts[0]
	targetID, err := strconv.Atoi(parts[1])
	if err != nil {
		return b.answerCallback(callback.ID, "Некорректная цель.")
	}
	if actor.ID == targetID {
		return b.answerCallback(callback.ID, "Нельзя взаимодействовать с собой.")
	}

	switch action {
	case "like", "dislike":
		if err := b.processRating(ctx, actor, targetID, action); err != nil {
			return b.answerCallback(callback.ID, err.Error())
		}
		return b.answerCallback(callback.ID, "Оценка учтена.")
	case "transfer":
		if len(parts) < 3 {
			return b.answerCallback(callback.ID, "Укажите сумму перевода.")
		}
		amount, err := strconv.Atoi(parts[2])
		if err != nil || amount <= 0 {
			return b.answerCallback(callback.ID, "Некорректная сумма.")
		}
		if err := b.processTransfer(ctx, actor, targetID, amount); err != nil {
			return b.answerCallback(callback.ID, err.Error())
		}
		return b.answerCallback(callback.ID, "Перевод выполнен.")
	default:
		return b.answerCallback(callback.ID, "Неизвестное действие.")
	}
}

func (b *Bot) handleStart(ctx context.Context, message *tgbotapi.Message) error {
	payload := strings.TrimSpace(message.CommandArguments())
	if strings.HasPrefix(payload, "player_") {
		linkHash := strings.TrimPrefix(payload, "player_")
		return b.showPlayerProfile(ctx, message.Chat.ID, message.From, linkHash)
	}

	player, err := b.ensurePlayer(ctx, message.From)
	if err != nil {
		return b.reply(message.Chat.ID, "Не удалось создать игрока. Попробуйте позже.")
	}
	return b.reply(message.Chat.ID, fmt.Sprintf("Привет, %s! Ваш уровень: %d, рейтинг: %d", player.FullName, player.Level, player.Rating))
}

func (b *Bot) handleRegister(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) == 0 {
		return b.reply(message.Chat.ID, "Формат: /register [роль] <полное имя>")
	}

	role := rolePlayer
	fullNameArgs := args
	if len(args) > 1 && isRole(args[0]) {
		role = args[0]
		fullNameArgs = args[1:]
	}

	fullName := strings.Join(fullNameArgs, " ")
	if fullName == "" {
		return b.reply(message.Chat.ID, "Укажите имя персонажа.")
	}

	player, err := b.ensurePlayer(ctx, message.From)
	if err != nil {
		return b.reply(message.Chat.ID, "Не удалось зарегистрировать игрока.")
	}

	if err := b.store.UpdatePlayerProfile(ctx, player.Telegram, fullName, role); err != nil {
		return b.reply(message.Chat.ID, "Не удалось обновить анкету.")
	}

	return b.reply(message.Chat.ID, fmt.Sprintf("Анкета обновлена: %s (%s)", fullName, role))
}

func (b *Bot) handleMyLink(ctx context.Context, message *tgbotapi.Message) error {
	player, err := b.ensurePlayer(ctx, message.From)
	if err != nil {
		return b.reply(message.Chat.ID, "Не удалось получить профиль.")
	}
	linkHash, err := b.store.GetPlayerLink(ctx, player.ID)
	if err != nil {
		linkHash, err = b.store.CreatePlayerLink(ctx, player.ID)
		if err != nil {
			return b.reply(message.Chat.ID, "Не удалось создать ссылку.")
		}
	}
	link := b.buildPlayerLink(linkHash)
	qrPNG, err := qrcode.Encode(link, qrcode.Medium, 256)
	if err != nil {
		return b.reply(message.Chat.ID, fmt.Sprintf("Ваша ссылка: %s", link))
	}
	photo := tgbotapi.NewPhoto(message.Chat.ID, tgbotapi.FileBytes{Name: "qr.png", Bytes: qrPNG})
	photo.Caption = fmt.Sprintf("Ваша ссылка: %s", link)
	_, err = b.api.Send(photo)
	return err
}

func (b *Bot) buildPlayerLink(linkHash string) string {
	return fmt.Sprintf("%s?start=player_%s", b.botLinkBase, linkHash)
}

func (b *Bot) handleAddPlayer(ctx context.Context, message *tgbotapi.Message) error {
	if err := b.requireAdmin(ctx, message.From.ID, message.Chat.ID); err != nil {
		return err
	}
	args := strings.Fields(message.CommandArguments())
	if len(args) < 2 {
		return b.reply(message.Chat.ID, "Формат: /add_player <telegram_id> <полное имя>")
	}
	telegramID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return b.reply(message.Chat.ID, "Некорректный telegram_id.")
	}
	fullName := strings.Join(args[1:], " ")
	player, err := b.store.CreatePlayer(ctx, telegramID, "", fullName)
	if err != nil {
		return b.reply(message.Chat.ID, "Не удалось создать игрока.")
	}
	linkHash, err := b.store.CreatePlayerLink(ctx, player.ID)
	if err != nil {
		return b.reply(message.Chat.ID, "Игрок создан, но ссылка не сгенерирована.")
	}
	return b.reply(message.Chat.ID, fmt.Sprintf("Игрок создан. Ссылка: %s", b.buildPlayerLink(linkHash)))
}

func (b *Bot) handleSetCycleDuration(ctx context.Context, message *tgbotapi.Message) error {
	if err := b.requireAdmin(ctx, message.From.ID, message.Chat.ID); err != nil {
		return err
	}
	minutes, err := strconv.Atoi(strings.TrimSpace(message.CommandArguments()))
	if err != nil || minutes < 15 {
		return b.reply(message.Chat.ID, "Укажите длительность в минутах (>= 15).")
	}
	if err := b.store.UpdateCycleDuration(ctx, minutes); err != nil {
		return b.reply(message.Chat.ID, "Не удалось обновить длительность цикла.")
	}
	return b.reply(message.Chat.ID, fmt.Sprintf("Длительность цикла обновлена: %d мин.", minutes))
}

func (b *Bot) handleSetRatingTimeout(ctx context.Context, message *tgbotapi.Message) error {
	if err := b.requireAdmin(ctx, message.From.ID, message.Chat.ID); err != nil {
		return err
	}
	minutes, err := strconv.Atoi(strings.TrimSpace(message.CommandArguments()))
	if err != nil || minutes <= 0 {
		return b.reply(message.Chat.ID, "Укажите таймаут в минутах (> 0).")
	}
	if err := b.store.UpdateRatingTimeout(ctx, minutes); err != nil {
		return b.reply(message.Chat.ID, "Не удалось обновить таймаут оценок.")
	}
	return b.reply(message.Chat.ID, fmt.Sprintf("Таймаут оценок обновлен: %d мин.", minutes))
}

func (b *Bot) handleSetRatingLimits(ctx context.Context, message *tgbotapi.Message) error {
	if err := b.requireAdmin(ctx, message.From.ID, message.Chat.ID); err != nil {
		return err
	}
	args := strings.Fields(message.CommandArguments())
	if len(args) != 2 {
		return b.reply(message.Chat.ID, "Формат: /set_rating_limits <уровень 1-5> <лимит>")
	}
	level, err := strconv.Atoi(args[0])
	if err != nil || level < 1 || level > 5 {
		return b.reply(message.Chat.ID, "Уровень должен быть от 1 до 5.")
	}
	limit, err := strconv.Atoi(args[1])
	if err != nil || limit <= 0 {
		return b.reply(message.Chat.ID, "Лимит должен быть больше 0.")
	}
	if err := b.store.UpsertRatingLimit(ctx, level, limit); err != nil {
		return b.reply(message.Chat.ID, "Не удалось обновить лимит.")
	}
	return b.reply(message.Chat.ID, fmt.Sprintf("Лимит для уровня %d обновлен: %d.", level, limit))
}

func (b *Bot) handleSetLevelBoundary(ctx context.Context, message *tgbotapi.Message) error {
	if err := b.requireAdmin(ctx, message.From.ID, message.Chat.ID); err != nil {
		return err
	}
	args := strings.Fields(message.CommandArguments())
	if len(args) != 3 {
		return b.reply(message.Chat.ID, "Формат: /set_level_boundary <уровень 1-5> <мин рейтинг> <макс рейтинг>")
	}
	level, err := strconv.Atoi(args[0])
	if err != nil || level < 1 || level > 5 {
		return b.reply(message.Chat.ID, "Уровень должен быть от 1 до 5.")
	}
	minRating, err := strconv.Atoi(args[1])
	if err != nil {
		return b.reply(message.Chat.ID, "Некорректный минимум рейтинга.")
	}
	maxRating, err := strconv.Atoi(args[2])
	if err != nil || maxRating < minRating {
		return b.reply(message.Chat.ID, "Максимум должен быть больше минимума.")
	}
	cfg, err := b.store.GetSystemConfig(ctx)
	if err != nil {
		return b.reply(message.Chat.ID, "Не удалось получить настройки.")
	}
	cycle, err := b.store.EnsureActiveCycle(ctx, cfg)
	if err != nil {
		return b.reply(message.Chat.ID, "Не удалось получить цикл.")
	}
	if err := b.store.SetLevelBoundary(ctx, cycle.ID, level, minRating, maxRating); err != nil {
		return b.reply(message.Chat.ID, "Не удалось сохранить границы.")
	}
	return b.reply(message.Chat.ID, fmt.Sprintf("Границы уровня %d обновлены: %d-%d", level, minRating, maxRating))
}

func (b *Bot) handleApplyLevelRecalc(ctx context.Context, message *tgbotapi.Message) error {
	if err := b.requireAdmin(ctx, message.From.ID, message.Chat.ID); err != nil {
		return err
	}
	cfg, err := b.store.GetSystemConfig(ctx)
	if err != nil {
		return b.reply(message.Chat.ID, "Не удалось получить настройки.")
	}
	cycle, err := b.store.EnsureActiveCycle(ctx, cfg)
	if err != nil {
		return b.reply(message.Chat.ID, "Не удалось получить цикл.")
	}
	boundaries, err := b.store.GetLevelBoundaries(ctx, cycle.ID)
	if err != nil {
		return b.reply(message.Chat.ID, "Не удалось получить границы.")
	}
	if len(boundaries) == 0 {
		return b.reply(message.Chat.ID, "Границы уровней не заданы.")
	}
	if err := b.store.RecalculateLevels(ctx, cycle.ID, boundaries); err != nil {
		return b.reply(message.Chat.ID, "Пересчет уровней не удался.")
	}
	return b.reply(message.Chat.ID, "Пересчет уровней завершен.")
}

func (b *Bot) handleCreateAdmin(ctx context.Context, message *tgbotapi.Message) error {
	hasAnyAdmin, err := b.store.HasAnyAdmin(ctx)
	if err != nil {
		return b.reply(message.Chat.ID, "Не удалось проверить список администраторов.")
	}

	if hasAnyAdmin {
		if err := b.requireAdmin(ctx, message.From.ID, message.Chat.ID); err != nil {
			return err
		}
	}

	telegramID, err := strconv.ParseInt(strings.TrimSpace(message.CommandArguments()), 10, 64)
	if err != nil {
		return b.reply(message.Chat.ID, "Укажите корректный telegram_id.")
	}

	if !hasAnyAdmin {
		if message.From == nil || telegramID != message.From.ID {
			return b.reply(message.Chat.ID, "Первого администратора можно назначить только на себя: /create_admin <ваш_telegram_id>.")
		}
		if err := b.store.SetPlayerRole(ctx, telegramID, roleSuperAdmin); err != nil {
			return b.reply(message.Chat.ID, "Не удалось назначить первого администратора. Сначала выполните /start.")
		}
		return b.reply(message.Chat.ID, "Вы назначены первым администратором (super_admin).")
	}

	if err := b.store.SetPlayerRole(ctx, telegramID, roleAdmin); err != nil {
		return b.reply(message.Chat.ID, "Не удалось назначить администратора.")
	}
	return b.reply(message.Chat.ID, "Администратор назначен.")
}

func (b *Bot) handleTransfer(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) != 2 {
		return b.reply(message.Chat.ID, "Формат: /transfer <telegram_id> <сумма>")
	}
	telegramID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return b.reply(message.Chat.ID, "Некорректный telegram_id.")
	}
	amount, err := strconv.Atoi(args[1])
	if err != nil || amount <= 0 {
		return b.reply(message.Chat.ID, "Сумма должна быть больше 0.")
	}

	sender, err := b.ensurePlayer(ctx, message.From)
	if err != nil {
		return b.reply(message.Chat.ID, "Не удалось определить отправителя.")
	}
	receiver, err := b.store.GetPlayerByTelegramID(ctx, telegramID)
	if err != nil {
		return b.reply(message.Chat.ID, "Получатель не найден.")
	}
	if sender.ID == receiver.ID {
		return b.reply(message.Chat.ID, "Нельзя переводить себе.")
	}
	if err := b.processTransferWithPlayers(ctx, sender, receiver, amount); err != nil {
		return b.reply(message.Chat.ID, err.Error())
	}
	return b.reply(message.Chat.ID, "Перевод выполнен.")
}

func (b *Bot) processRating(ctx context.Context, actor db.Player, targetID int, ratingType string) error {
	target, err := b.store.GetPlayerByID(ctx, targetID)
	if err != nil {
		return errors.New("Игрок не найден.")
	}

	cfg, err := b.store.GetSystemConfig(ctx)
	if err != nil {
		return errors.New("Настройки недоступны.")
	}
	cycle, err := b.store.EnsureActiveCycle(ctx, cfg)
	if err != nil {
		return errors.New("Не удалось получить цикл.")
	}

	lastRatingAt, err := b.store.GetLastRatingBetween(ctx, actor.ID, target.ID)
	if err == nil {
		if time.Since(lastRatingAt) < time.Duration(cycle.RatingTimeoutMinutes)*time.Minute {
			return fmt.Errorf("Слишком частая оценка. Попробуйте позже.")
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return errors.New("Не удалось проверить таймаут.")
	}

	limit, err := b.store.GetRatingLimit(ctx, actor.Level)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return errors.New("Не удалось проверить лимиты.")
	}
	if err == nil {
		count, err := b.store.CountRatingsByRaterInCycle(ctx, actor.ID, cycle.ID)
		if err != nil {
			return errors.New("Не удалось проверить лимиты.")
		}
		if count >= limit.Limit {
			return errors.New("Лимит оценок за цикл исчерпан.")
		}
	}

	ratingChange := calculateRatingChange(actor.Level, target.Level, cfg, ratingType)
	result, err := b.store.CreateRating(ctx, actor, target, cycle, ratingType, ratingChange)
	if err != nil {
		return errors.New("Не удалось сохранить оценку.")
	}

	details := map[string]any{
		"rating_change": result.RatingChange,
		"rater_level":   actor.Level,
		"rated_level":   target.Level,
	}
	payload, _ := json.Marshal(details)
	_ = b.store.LogOperation(ctx, "rating_"+ratingType, &actor.ID, &target.ID, payload)
	return nil
}

func (b *Bot) processTransfer(ctx context.Context, actor db.Player, targetID int, amount int) error {
	target, err := b.store.GetPlayerByID(ctx, targetID)
	if err != nil {
		return errors.New("Игрок не найден.")
	}
	return b.processTransferWithPlayers(ctx, actor, target, amount)
}

func (b *Bot) processTransferWithPlayers(ctx context.Context, sender db.Player, receiver db.Player, amount int) error {
	if sender.Rating < amount {
		return errors.New("Недостаточно рейтинга.")
	}
	cfg, err := b.store.GetSystemConfig(ctx)
	if err != nil {
		return errors.New("Настройки недоступны.")
	}
	cycle, err := b.store.EnsureActiveCycle(ctx, cfg)
	if err != nil {
		return errors.New("Не удалось получить цикл.")
	}
	if err := b.store.CreateTransfer(ctx, sender, receiver, cycle.ID, amount, "manual transfer"); err != nil {
		return errors.New("Перевод не удался.")
	}
	details := map[string]any{"amount": amount}
	payload, _ := json.Marshal(details)
	_ = b.store.LogOperation(ctx, "rating_transfer", &sender.ID, &receiver.ID, payload)
	return nil
}

func (b *Bot) showPlayerProfile(ctx context.Context, chatID int64, from *tgbotapi.User, linkHash string) error {
	viewer, err := b.ensurePlayer(ctx, from)
	if err != nil {
		return b.reply(chatID, "Не удалось определить игрока.")
	}
	target, err := b.store.GetPlayerByLinkHash(ctx, linkHash)
	if err != nil {
		return b.reply(chatID, "Ссылка не найдена.")
	}
	if viewer.ID == target.ID {
		return b.reply(chatID, fmt.Sprintf("Это ваша карточка: %s (уровень %d, рейтинг %d)", target.FullName, target.Level, target.Rating))
	}

	text := fmt.Sprintf("%s\nУровень: %d\nРейтинг: %d", target.FullName, target.Level, target.Rating)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = profileKeyboard(target.ID)
	_, err = b.api.Send(msg)
	return err
}

func (b *Bot) ensurePlayer(ctx context.Context, user *tgbotapi.User) (db.Player, error) {
	player, err := b.store.GetPlayerByTelegramID(ctx, user.ID)
	if err == nil {
		return player, nil
	}
	fullName := strings.TrimSpace(strings.TrimSpace(user.FirstName + " " + user.LastName))
	if fullName == "" {
		fullName = user.UserName
	}
	return b.store.CreatePlayer(ctx, user.ID, user.UserName, fullName)
}

func (b *Bot) requireAdmin(ctx context.Context, telegramID int64, chatID int64) error {
	isAdmin, err := b.store.IsAdmin(ctx, telegramID)
	if err != nil {
		return b.reply(chatID, "Не удалось проверить права.")
	}
	if !isAdmin {
		return b.reply(chatID, "Недостаточно прав.")
	}
	return nil
}

func (b *Bot) reply(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := b.api.Send(msg)
	if err != nil {
		b.log.Error("send message failed", "chat_id", chatID, "error", err)
		return err
	}
	b.log.Info("message sent", "chat_id", chatID, "text_len", len(text))
	return err
}

func (b *Bot) answerCallback(callbackID, text string) error {
	response := tgbotapi.NewCallback(callbackID, text)
	_, err := b.api.Request(response)
	if err != nil {
		b.log.Error("answer callback failed", "callback_id", callbackID, "error", err)
		return err
	}
	b.log.Info("callback answered", "callback_id", callbackID, "text_len", len(text))
	return err
}

func calculateRatingChange(raterLevel, ratedLevel int, cfg db.SystemConfig, ratingType string) int {
	z := 1.0
	if ratingType == "dislike" {
		z = -1.0
	}
	result := z * (cfg.RatingFormulaA * float64(raterLevel)) / (float64(ratedLevel) * cfg.RatingFormulaB)
	rounded := int(math.Round(result))
	if rounded == 0 {
		if ratingType == "like" {
			return 1
		}
		return -1
	}
	return rounded
}

func profileKeyboard(targetID int) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👍 Лайк", fmt.Sprintf("like:%d", targetID)),
			tgbotapi.NewInlineKeyboardButtonData("👎 Дизлайк", fmt.Sprintf("dislike:%d", targetID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Перевод +1", fmt.Sprintf("transfer:%d:1", targetID)),
			tgbotapi.NewInlineKeyboardButtonData("Перевод +5", fmt.Sprintf("transfer:%d:5", targetID)),
		),
	)
}

func isRole(value string) bool {
	switch value {
	case rolePlayer, roleModerator, roleAdmin, roleSuperAdmin:
		return true
	default:
		return false
	}
}
