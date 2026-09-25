package network

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	tea "github.com/charmbracelet/bubbletea"
)

// --- Messages ---

// NewBlockMsg est envoyé à l'UI à chaque nouveau bloc Substrate.
type NewBlockMsg struct {
	Number int
	Hash   string
}

// BlockWatchErrMsg signale une erreur du block watcher dans les logs UI.
type BlockWatchErrMsg struct {
	Err error
}

// BlockSub est le channel de communication entre la goroutine WebSocket et Bubbletea.
type BlockSub chan tea.Msg

// --- Bubbletea integration ---

// NewBlockSub crée le channel partagé. À appeler une seule fois dans InitialModel().
func NewBlockSub() BlockSub {
	return make(BlockSub, 1)
}

// WaitForBlock retourne un tea.Cmd qui se bloque jusqu'au prochain message du block watcher.
// Doit être re-queued dans Update() à chaque réception pour continuer à écouter.
func WaitForBlock(sub BlockSub) tea.Cmd {
	return func() tea.Msg {
		return <-sub
	}
}

// StartBlockWatcher lance la goroutine long-lived qui maintient la connexion WebSocket
// et envoie les NewBlockMsg (ou BlockWatchErrMsg) sur le channel sub.
func StartBlockWatcher(wsURL string, sub BlockSub) {
	go func() {
		backoff := 1 * time.Second
		for {
			err := connectAndSubscribe(wsURL, sub)
			if err != nil {
				// Envoie l'erreur à l'UI pour affichage dans les logs
				select {
				case sub <- BlockWatchErrMsg{Err: fmt.Errorf("block watcher: %w", err)}:
				default:
				}
			}
			// Backoff exponentiel plafonné à 30s
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}()
}

// --- WebSocket Substrate logic ---

// substrateNotification représente le format JSON-RPC push du nœud Substrate.
type substrateNotification struct {
	Method string `json:"method"`
	Params struct {
		Result struct {
			Number     string `json:"number"`
			ParentHash string `json:"parentHash"`
		} `json:"result"`
	} `json:"params"`
}

func connectAndSubscribe(wsURL string, sub BlockSub) error {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", wsURL, err)
	}
	defer conn.Close()

	// Souscription à chain_subscribeNewHeads
	subscribeReq := `{"id":1,"jsonrpc":"2.0","method":"chain_subscribeNewHeads","params":[]}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(subscribeReq)); err != nil {
		return fmt.Errorf("write subscribe: %w", err)
	}

	// Lire la confirmation de souscription (on l'ignore, on attend juste un 200 implicite)
	if _, _, err := conn.ReadMessage(); err != nil {
		return fmt.Errorf("read subscription ack: %w", err)
	}

	// Reset backoff : connexion établie avec succès
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read message: %w", err)
		}

		var notif substrateNotification
		if err := json.Unmarshal(raw, &notif); err != nil {
			continue // message inattendu, on ignore
		}

		if notif.Method != "chain_newHead" {
			continue
		}

		blockNum, err := hexToInt(notif.Params.Result.Number)
		if err != nil {
			continue
		}

		// Envoi non-bloquant : on drop si l'UI n'a pas encore consommé le bloc précédent.
		// La prochaine notification Substrate couvrira le bloc manqué.
		select {
		case sub <- NewBlockMsg{Number: blockNum, Hash: notif.Params.Result.ParentHash}:
		default:
		}
	}
}

// hexToInt convertit un hex Substrate ("0x4288A1") en int.
func hexToInt(h string) (int, error) {
	h = strings.TrimPrefix(strings.TrimPrefix(h, "0x"), "0X")
	val, err := strconv.ParseInt(h, 16, 64)
	return int(val), err
}
