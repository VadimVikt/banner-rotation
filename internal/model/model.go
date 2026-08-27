package model

import "time"

// Slot представляет рекламный слот на странице.
type Slot struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// Banner представляет рекламный баннер.
type Banner struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// SlotBanner описывает связь между слотом и баннером.
// Один баннер может быть привязан к нескольким слотам (связь многие-ко-многим).
// Это просто структура-данные, она не содержит методов работы с БД.
type SlotBanner struct {
	SlotID   string `json:"slotId"`
	BannerID string `json:"bannerId"`
}

// GroupID используется для сегментации статистики (например, по географии или типу устройства).
// Алиас введен для самодокументируемого кода вместо использования голого string.
type GroupID string

// BannerStats хранит агрегированную статистику показов и кликов.
// Данные собираются для конкретной комбинации: Слот + Баннер + Группа.
type BannerStats struct {
	SlotID      string  `json:"slotId"`
	BannerID    string  `json:"bannerId"`
	GroupID     GroupID `json:"groupId"`
	Impressions int     `json:"impressions"` // Количество показов
	Clicks      int     `json:"clicks"`      // Количество кликов
}

// EventType определяет допустимые типы событий трекера.
// Использование собственного типа вместо string повышает типобезопасность.
type EventType string

const (
	EventTypeClick      EventType = "click"
	EventTypeImpression EventType = "impression"
)

// Event представляет единичное событие от пользователя.
type Event struct {
	Type      EventType `json:"type"`
	SlotID    string    `json:"slotId"`
	BannerID  string    `json:"bannerId"`
	GroupID   GroupID   `json:"groupId"`
	Timestamp time.Time `json:"timestamp"`
}
