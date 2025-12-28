package admin

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"rts_for_rating_on_larp/internal/db"
)

type Handler struct {
	store      *db.Store
	adminToken string
	tpl        *template.Template
}

type viewData struct {
	Message string
	Error   string
}

func New(store *db.Store, adminToken string) (*Handler, error) {
	tpl, err := template.New("admin").Parse(adminTemplate)
	if err != nil {
		return nil, err
	}
	return &Handler{store: store, adminToken: adminToken, tpl: tpl}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(r) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthorized"))
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/admin":
		h.render(w, viewData{})
	case r.Method == http.MethodPost && r.URL.Path == "/admin/action":
		h.handleAction(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (h *Handler) authorize(r *http.Request) bool {
	if h.adminToken == "" {
		return false
	}
	if token := r.Header.Get("X-Admin-Token"); token != "" {
		return token == h.adminToken
	}
	return r.URL.Query().Get("token") == h.adminToken
}

func (h *Handler) handleAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.render(w, viewData{Error: "Некорректные данные."})
		return
	}
	ctx := r.Context()
	action := r.FormValue("action")

	var err error
	var message string
	switch action {
	case "set_cycle_duration":
		minutes, convErr := strconv.Atoi(strings.TrimSpace(r.FormValue("minutes")))
		if convErr != nil || minutes < 15 {
			err = errors.New("Длительность должна быть >= 15")
			break
		}
		err = h.store.UpdateCycleDuration(ctx, minutes)
		message = fmt.Sprintf("Длительность цикла обновлена: %d мин.", minutes)
	case "set_rating_timeout":
		minutes, convErr := strconv.Atoi(strings.TrimSpace(r.FormValue("minutes")))
		if convErr != nil || minutes <= 0 {
			err = errors.New("Таймаут должен быть > 0")
			break
		}
		err = h.store.UpdateRatingTimeout(ctx, minutes)
		message = fmt.Sprintf("Таймаут оценок обновлен: %d мин.", minutes)
	case "set_rating_limit":
		level, levelErr := strconv.Atoi(strings.TrimSpace(r.FormValue("level")))
		limit, limitErr := strconv.Atoi(strings.TrimSpace(r.FormValue("limit")))
		if levelErr != nil || level < 1 || level > 5 || limitErr != nil || limit <= 0 {
			err = errors.New("Некорректные параметры уровня или лимита")
			break
		}
		err = h.store.UpsertRatingLimit(ctx, level, limit)
		message = fmt.Sprintf("Лимит для уровня %d обновлен: %d", level, limit)
	case "set_level_boundary":
		level, levelErr := strconv.Atoi(strings.TrimSpace(r.FormValue("level")))
		minRating, minErr := strconv.Atoi(strings.TrimSpace(r.FormValue("min_rating")))
		maxRating, maxErr := strconv.Atoi(strings.TrimSpace(r.FormValue("max_rating")))
		if levelErr != nil || level < 1 || level > 5 || minErr != nil || maxErr != nil || maxRating < minRating {
			err = errors.New("Некорректные границы уровня")
			break
		}
		cfg, cfgErr := h.store.GetSystemConfig(ctx)
		if cfgErr != nil {
			err = cfgErr
			break
		}
		cycle, cycleErr := h.store.EnsureActiveCycle(ctx, cfg)
		if cycleErr != nil {
			err = cycleErr
			break
		}
		err = h.store.SetLevelBoundary(ctx, cycle.ID, level, minRating, maxRating)
		message = fmt.Sprintf("Границы уровня %d обновлены: %d-%d", level, minRating, maxRating)
	case "apply_level_recalc":
		message, err = h.applyRecalc(ctx)
	case "add_player":
		telegramID, convErr := strconv.ParseInt(strings.TrimSpace(r.FormValue("telegram_id")), 10, 64)
		fullName := strings.TrimSpace(r.FormValue("full_name"))
		if convErr != nil || fullName == "" {
			err = errors.New("Некорректные данные игрока")
			break
		}
		player, createErr := h.store.CreatePlayer(ctx, telegramID, "", fullName)
		if createErr != nil {
			err = createErr
			break
		}
		_, _ = h.store.CreatePlayerLink(ctx, player.ID)
		message = fmt.Sprintf("Игрок создан: %s", fullName)
	case "create_admin":
		telegramID, convErr := strconv.ParseInt(strings.TrimSpace(r.FormValue("telegram_id")), 10, 64)
		if convErr != nil {
			err = errors.New("Некорректный telegram_id")
			break
		}
		err = h.store.SetPlayerRole(ctx, telegramID, "admin")
		message = "Администратор назначен."
	default:
		err = errors.New("Неизвестное действие")
	}

	if err != nil {
		h.render(w, viewData{Error: err.Error()})
		return
	}
	h.render(w, viewData{Message: message})
}

func (h *Handler) applyRecalc(ctx context.Context) (string, error) {
	cfg, err := h.store.GetSystemConfig(ctx)
	if err != nil {
		return "", err
	}
	cycle, err := h.store.EnsureActiveCycle(ctx, cfg)
	if err != nil {
		return "", err
	}
	boundaries, err := h.store.GetLevelBoundaries(ctx, cycle.ID)
	if err != nil {
		return "", err
	}
	if len(boundaries) == 0 {
		return "", errors.New("Границы уровней не заданы")
	}
	if err := h.store.RecalculateLevels(ctx, cycle.ID, boundaries); err != nil {
		return "", err
	}
	return "Пересчет уровней завершен.", nil
}

func (h *Handler) render(w http.ResponseWriter, data viewData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tpl.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

const adminTemplate = `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8" />
  <title>Новый Рим — Админка</title>
  <style>
    body { font-family: sans-serif; margin: 2rem; }
    fieldset { margin-bottom: 1.5rem; padding: 1rem; }
    label { display: block; margin-top: 0.5rem; }
    input { padding: 0.4rem; width: 280px; }
    button { margin-top: 0.75rem; padding: 0.5rem 1rem; }
    .message { color: #0b5; }
    .error { color: #b00; }
  </style>
</head>
<body>
  <h1>Админка Новый Рим</h1>
  {{if .Message}}<p class="message">{{.Message}}</p>{{end}}
  {{if .Error}}<p class="error">{{.Error}}</p>{{end}}

  <form method="post" action="/admin/action">
    <fieldset>
      <legend>Циклы</legend>
      <input type="hidden" name="action" value="set_cycle_duration" />
      <label>Длительность (минуты)
        <input name="minutes" type="number" min="15" required />
      </label>
      <button type="submit">Обновить длительность</button>
    </fieldset>
  </form>

  <form method="post" action="/admin/action">
    <fieldset>
      <legend>Таймаут оценок</legend>
      <input type="hidden" name="action" value="set_rating_timeout" />
      <label>Таймаут (минуты)
        <input name="minutes" type="number" min="1" required />
      </label>
      <button type="submit">Обновить таймаут</button>
    </fieldset>
  </form>

  <form method="post" action="/admin/action">
    <fieldset>
      <legend>Лимит оценок</legend>
      <input type="hidden" name="action" value="set_rating_limit" />
      <label>Уровень (1-5)
        <input name="level" type="number" min="1" max="5" required />
      </label>
      <label>Лимит
        <input name="limit" type="number" min="1" required />
      </label>
      <button type="submit">Обновить лимит</button>
    </fieldset>
  </form>

  <form method="post" action="/admin/action">
    <fieldset>
      <legend>Границы уровней</legend>
      <input type="hidden" name="action" value="set_level_boundary" />
      <label>Уровень (1-5)
        <input name="level" type="number" min="1" max="5" required />
      </label>
      <label>Мин. рейтинг
        <input name="min_rating" type="number" required />
      </label>
      <label>Макс. рейтинг
        <input name="max_rating" type="number" required />
      </label>
      <button type="submit">Сохранить границы</button>
    </fieldset>
  </form>

  <form method="post" action="/admin/action">
    <fieldset>
      <legend>Пересчет уровней</legend>
      <input type="hidden" name="action" value="apply_level_recalc" />
      <button type="submit">Запустить пересчет</button>
    </fieldset>
  </form>

  <form method="post" action="/admin/action">
    <fieldset>
      <legend>Добавить игрока</legend>
      <input type="hidden" name="action" value="add_player" />
      <label>Telegram ID
        <input name="telegram_id" type="number" required />
      </label>
      <label>Полное имя
        <input name="full_name" type="text" required />
      </label>
      <button type="submit">Создать игрока</button>
    </fieldset>
  </form>

  <form method="post" action="/admin/action">
    <fieldset>
      <legend>Назначить администратора</legend>
      <input type="hidden" name="action" value="create_admin" />
      <label>Telegram ID
        <input name="telegram_id" type="number" required />
      </label>
      <button type="submit">Назначить</button>
    </fieldset>
  </form>
</body>
</html>`
