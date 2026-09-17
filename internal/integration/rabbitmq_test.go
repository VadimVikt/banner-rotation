// Package integration provides RabbitMQ integration tests with a real broker in Docker.
//
// The broker container is started once in TestMain for the whole test binary run,
// so `-count N` does not restart the container N times. Tests skip when Docker
// is unavailable.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/streadway/amqp"

	"github.com/VadimVikt/banner-rotation/internal/event"
	"github.com/VadimVikt/banner-rotation/internal/model"
)

const (
	rabbitmqImage      = "rabbitmq:3-management"
	rabbitmqPort       = 5678 // host port (avoid conflict with default 5672)
	containerName      = "banner-rotation-rabbitmq-test"
	readinessTimeout   = 60 * time.Second
	consumeTimeout     = 10 * time.Second
	eventsExchangeName = "banner_events" // must match event package default
)

var (
	rabbitmqAvailable bool
	rabbitmqURL       string
)

// TestMain starts the RabbitMQ container once for the entire test run.
func TestMain(m *testing.M) {
	if _, err := exec.LookPath("docker"); err == nil {
		if url, err := startRabbitMQContainer(); err == nil {
			rabbitmqURL = url
			rabbitmqAvailable = true
		}
	}
	code := m.Run()
	stopRabbitMQContainer()
	os.Exit(code)
}

func startRabbitMQContainer() (string, error) {
	url := fmt.Sprintf("amqp://guest:guest@localhost:%d/", rabbitmqPort)

	// Stop and remove any leftover container with the same name.
	exec.Command("docker", "rm", "-f", containerName).Run() // ignore error

	// Start RabbitMQ container.
	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"-p", fmt.Sprintf("%d:5672", rabbitmqPort),
		rabbitmqImage,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("docker run: %w (output: %s)", err, string(out))
	}

	// Wait for the broker to accept AMQP connections.
	deadline := time.Now().Add(readinessTimeout)
	for time.Now().Before(deadline) {
		pub, err := event.NewRabbitMQPublisher(url)
		if err == nil {
			pub.Close()
			return url, nil
		}
		time.Sleep(time.Second)
	}
	return "", fmt.Errorf("rabbitmq did not become ready within %s", readinessTimeout)
}

func stopRabbitMQContainer() {
	exec.Command("docker", "rm", "-f", containerName).Run() // ignore error
}

func requireRabbitMQ(t *testing.T) {
	t.Helper()
	if !rabbitmqAvailable {
		t.Skip("docker/RabbitMQ unavailable, skipping RabbitMQ integration test")
	}
}

// consumeOne declares a unique exclusive queue bound to the events exchange,
// publishes ev, and returns the first delivered message decoded as model.Event.
// Binding happens before publish so only this run's event is expected.
func consumeOne(t *testing.T, ev model.Event) model.Event {
	t.Helper()

	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		t.Fatalf("dial rabbitmq: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel: %v", err)
	}
	defer ch.Close()

	queue := fmt.Sprintf("test_consume_%d", time.Now().UnixNano())
	if _, err := ch.QueueDeclare(queue, false, true, true, false, nil); err != nil {
		t.Fatalf("declare consumer queue: %v", err)
	}
	if err := ch.QueueBind(queue, "#", eventsExchangeName, false, nil); err != nil {
		t.Fatalf("bind consumer queue: %v", err)
	}

	pub, err := event.NewRabbitMQPublisher(rabbitmqURL)
	if err != nil {
		t.Fatalf("create publisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Publish(context.Background(), ev); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	dels, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		t.Fatalf("consume from queue: %v", err)
	}

	select {
	case d := <-dels:
		var got model.Event
		if err := json.Unmarshal(d.Body, &got); err != nil {
			t.Fatalf("unmarshal delivered event: %v", err)
		}
		return got
	case <-time.After(consumeTimeout):
		t.Fatal("timed out waiting for delivered message")
	}
	return model.Event{}
}

// TestRabbitMQ_PublishAndConsume verifies that a published event is delivered
// to a queue bound to the events exchange with an intact JSON payload.
func TestRabbitMQ_PublishAndConsume(t *testing.T) {
	requireRabbitMQ(t)

	ev := model.Event{
		Type:      model.EventTypeImpression,
		SlotID:    "slot1",
		BannerID:  "banner1",
		GroupID:   model.GroupID("group1"),
		Timestamp: time.Now(),
	}

	got := consumeOne(t, ev)

	if got.Type != ev.Type || got.SlotID != ev.SlotID || got.BannerID != ev.BannerID || got.GroupID != ev.GroupID {
		t.Fatalf("delivered event mismatch: got %+v, want %+v", got, ev)
	}
}

// TestRabbitMQ_MultiplePublishes verifies that multiple events can be published successfully.
func TestRabbitMQ_MultiplePublishes(t *testing.T) {
	requireRabbitMQ(t)

	pub, err := event.NewRabbitMQPublisher(rabbitmqURL)
	if err != nil {
		t.Fatalf("failed to create RabbitMQ publisher: %v", err)
	}
	defer pub.Close()

	// Publish 10 events
	for i := 0; i < 10; i++ {
		ev := model.Event{
			Type:      model.EventTypeClick,
			SlotID:    "slot1",
			BannerID:  fmt.Sprintf("banner_%d", i),
			GroupID:   model.GroupID("group1"),
			Timestamp: time.Now(),
		}
		if err := pub.Publish(context.Background(), ev); err != nil {
			t.Fatalf("publish event #%d failed: %v", i, err)
		}
	}
}

// TestRabbitMQ_PublishDeserializesCorrectly verifies that an event round-trips
// through the broker: publish JSON, consume, decode, and compare all fields.
func TestRabbitMQ_PublishDeserializesCorrectly(t *testing.T) {
	requireRabbitMQ(t)

	ev := model.Event{
		Type:      model.EventTypeImpression,
		SlotID:    "test_slot",
		BannerID:  "test_banner",
		GroupID:   model.GroupID("test_group"),
		Timestamp: time.Date(2025, 9, 11, 12, 0, 0, 0, time.UTC),
	}

	got := consumeOne(t, ev)

	if got.Type != ev.Type || got.SlotID != ev.SlotID || got.BannerID != ev.BannerID ||
		got.GroupID != ev.GroupID || !got.Timestamp.Equal(ev.Timestamp) {
		t.Fatalf("event deserialization mismatch after round-trip: got %+v, want %+v", got, ev)
	}
}
