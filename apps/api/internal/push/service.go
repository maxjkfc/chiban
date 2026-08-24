package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/google/uuid"
)

// AllowedPushHostSuffixes prevents SSRF by restricting outbound push to verified providers.
var AllowedPushHostSuffixes = []string{
	"push.apple.com",
	"fcm.googleapis.com",
	"notify.windows.com",
	"push.services.mozilla.com",
}

// ValidateEndpoint checks if the push subscription endpoint is a secure HTTPS URL from an allowed provider.
func ValidateEndpoint(rawEndpoint string) error {
	u, err := url.Parse(rawEndpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}
	if u.Scheme != "https" {
		return errors.New("endpoint must use https scheme")
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range AllowedPushHostSuffixes {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return nil
		}
	}
	return fmt.Errorf("endpoint host %q is not in the allowed push provider list", host)
}

type VAPIDKeys struct {
	PublicKey  string
	PrivateKey string
	Subject    string // e.g. "mailto:admin@example.com"
}

type Payload struct {
	Title   string    `json:"title"`
	Body    string    `json:"body"`
	Icon    string    `json:"icon,omitempty"`
	URL     string    `json:"url"`
	GroupID uuid.UUID `json:"group_id"`
	Tag     string    `json:"tag,omitempty"`
}

type FocusChecker interface {
	IsDeviceFocused(userID uuid.UUID, deviceID string, groupID uuid.UUID) bool
}

type Service struct {
	store        *Store
	keys         VAPIDKeys
	focusChecker FocusChecker
	httpClient   *http.Client
	workerSem    chan struct{}
}

func NewService(store *Store, keys VAPIDKeys, focusChecker FocusChecker) *Service {
	return &Service{
		store:        store,
		keys:         keys,
		focusChecker: focusChecker,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		workerSem:    make(chan struct{}, 8), // Bounded concurrency (max 8 parallel outbound requests)
	}
}

// NotifyGroup sends push notifications asynchronously to eligible group members.
func (s *Service) NotifyGroup(ctx context.Context, groupID uuid.UUID, senderID uuid.UUID, payload Payload) {
	if s.keys.PublicKey == "" || s.keys.PrivateKey == "" {
		return // Push not configured, skip
	}

	// Detach context so caller cancellation does not terminate in-flight push delivery
	detachedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)

	go func() {
		defer cancel()
		subs, err := s.store.ListByGroupEligible(detachedCtx, groupID, senderID)
		if err != nil {
			slog.Error("push: failed to list eligible subscriptions", "group_id", groupID, "err", err)
			return
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			slog.Error("push: failed to marshal payload", "err", err)
			return
		}

		var wg sync.WaitGroup
		for _, sub := range subs {
			// Check if device currently holds an active focus lease for this group
			if s.focusChecker != nil && s.focusChecker.IsDeviceFocused(sub.UserID, sub.DeviceID, groupID) {
				continue
			}

			wg.Add(1)
			go func(sub Subscription) {
				defer wg.Done()
				s.workerSem <- struct{}{}
				defer func() { <-s.workerSem }()

				s.sendOne(detachedCtx, sub, payloadBytes)
			}(sub)
		}
		wg.Wait()
	}()
}

func (s *Service) sendOne(ctx context.Context, sub Subscription, payloadBytes []byte) {
	if err := ValidateEndpoint(sub.Endpoint); err != nil {
		slog.Warn("push: skipping invalid endpoint", "subscription_id", sub.ID, "err", err)
		return
	}

	sPush := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256DH,
			Auth:   sub.Auth,
		},
	}

	// Retry up to 2 times for transient errors
	var lastStatusCode int
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*200) * time.Millisecond)
		}

		res, err := webpush.SendNotificationWithContext(ctx, payloadBytes, sPush, &webpush.Options{
			Subscriber:      s.keys.Subject,
			VAPIDPublicKey:  s.keys.PublicKey,
			VAPIDPrivateKey: s.keys.PrivateKey,
			TTL:             86400,
			HTTPClient:      s.httpClient,
		})
		if err != nil {
			slog.Warn("push: send failed", "subscription_id", sub.ID, "attempt", attempt, "err", err)
			continue
		}
		defer res.Body.Close()

		lastStatusCode = res.StatusCode
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			return // Success
		}

		// Categorize error response
		switch res.StatusCode {
		case http.StatusNotFound, http.StatusGone:
			// Terminal subscription error: Browser unregistered or expired
			slog.Info("push: subscription terminal, pruning", "subscription_id", sub.ID, "status", res.StatusCode)
			_ = s.store.DeleteByID(ctx, sub.ID)
			return
		case http.StatusUnauthorized, http.StatusForbidden:
			// VAPID or application credential error: log alert, DO NOT delete subscriber
			slog.Error("push: critical VAPID credential error from push service", "status", res.StatusCode, "endpoint", sub.Endpoint)
			return
		case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
			// Transient error: retry loop continues
			continue
		default:
			slog.Warn("push: unhandled push service status code", "status", res.StatusCode, "subscription_id", sub.ID)
			return
		}
	}
	slog.Warn("push: exhausted retries for subscription", "subscription_id", sub.ID, "last_status", lastStatusCode)
}
