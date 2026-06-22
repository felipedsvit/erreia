package board

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Notifier is implemented by realtime.Publisher; we keep a tiny interface
// here so the service doesn't import the realtime package directly.
type Notifier interface {
	Notify(ctx context.Context, payload string) error
}

type Service struct {
	repo     *Repo
	notifier Notifier
}

func NewService(pool *pgxpool.Pool, notifier Notifier) *Service {
	return &Service{repo: NewRepo(pool), notifier: notifier}
}

// GetColumn returns a single column with its cards.
func (s *Service) GetColumn(ctx context.Context, columnID string) (*Column, error) {
	return s.repo.GetColumn(ctx, columnID)
}

// GetBoard returns the full board tree (columns + cards) ordered by position.
func (s *Service) GetBoard(ctx context.Context, boardID string) (*Board, error) {
	return s.repo.GetBoard(ctx, boardID)
}

// BoardOwnerOf returns the board owner (used for authorization).
func (s *Service) BoardOwnerOf(ctx context.Context, boardID string) (string, error) {
	return s.repo.BoardOwnerOf(ctx, boardID)
}

// ColumnBoardID resolves a column to its board.
func (s *Service) ColumnBoardID(ctx context.Context, columnID string) (string, error) {
	return s.repo.ColumnBoardID(ctx, columnID)
}

// CardBoardID resolves a card to its board.
func (s *Service) CardBoardID(ctx context.Context, cardID string) (string, error) {
	return s.repo.CardBoardID(ctx, cardID)
}

// GetCard returns a single card.
func (s *Service) GetCard(ctx context.Context, cardID string) (*Card, error) {
	return s.repo.GetCard(ctx, cardID)
}

// CreateBoard persists a new board (with default columns) and notifies.
func (s *Service) CreateBoard(ctx context.Context, ownerID, title string) (*Board, error) {
	b, err := s.repo.CreateBoard(ctx, ownerID, title)
	if err != nil {
		return nil, err
	}
	s.notify(ctx, b.ID, "board-created", "", "")
	return b, nil
}

// CreateColumn appends a column to a board and notifies.
func (s *Service) CreateColumn(ctx context.Context, boardID, title string) (*Column, error) {
	c, err := s.repo.CreateColumn(ctx, boardID, title)
	if err != nil {
		return nil, err
	}
	s.notify(ctx, boardID, "column-created", "", c.ID)
	return c, nil
}

// CreateCard appends a card to a column and notifies.
func (s *Service) CreateCard(ctx context.Context, columnID, title string) (*Card, string, error) {
	card, err := s.repo.CreateCard(ctx, columnID, title)
	if err != nil {
		return nil, "", err
	}
	boardID, err := s.repo.ColumnBoardID(ctx, columnID)
	if err != nil {
		return nil, "", err
	}
	s.notify(ctx, boardID, "card-created", card.ID, columnID)
	return card, boardID, nil
}

// UpdateCard edits a card and notifies.
func (s *Service) UpdateCard(ctx context.Context, cardID, title, description string) (*Card, error) {
	card, err := s.repo.UpdateCard(ctx, cardID, title, description)
	if err != nil {
		return nil, err
	}
	boardID, err := s.repo.CardBoardID(ctx, cardID)
	if err != nil {
		return nil, err
	}
	s.notify(ctx, boardID, "card-updated", cardID, card.ColumnID)
	return card, nil
}

// DeleteCard removes a card and notifies.
func (s *Service) DeleteCard(ctx context.Context, cardID string) error {
	boardID, err := s.repo.CardBoardID(ctx, cardID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteCard(ctx, cardID); err != nil {
		return err
	}
	s.notify(ctx, boardID, "card-deleted", cardID, "")
	return nil
}

// MoveCard relocates a card and notifies. The version field on the event
// lets clients ignore stale events.
func (s *Service) MoveCard(ctx context.Context, cardID, destColumnID string, destPos int) (*Card, error) {
	card, boardID, err := s.repo.MoveCard(ctx, cardID, destColumnID, destPos)
	if err != nil {
		return nil, err
	}
	s.notify(ctx, boardID, "card-moved", cardID, destColumnID)
	return card, nil
}

// notify sends a NOTIFY via the configured notifier, failing soft: a broker
// outage must never roll back a committed database change.
func (s *Service) notify(ctx context.Context, boardID, action, cardID, columnID string) {
	payload := fmt.Sprintf(`{"b":%q,"a":%q,"id":%q,"c":%q}`, boardID, action, cardID, columnID)
	if err := s.notifier.Notify(ctx, payload); err != nil {
		// non-fatal: log and move on. The DB row is already correct.
		_ = err
	}
}
