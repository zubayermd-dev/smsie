package worker

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zubayermd-dev/ivy/internal/config"
	"github.com/zubayermd-dev/ivy/internal/model"
	"github.com/zubayermd-dev/ivy/internal/repository"
	"github.com/zubayermd-dev/ivy/pkg/logger"
	"github.com/warthog618/sms"
	"github.com/warthog618/sms/encoding/tpdu"
)

func (w *ModemWorker) logicLoop() {
	// Wait for init
	time.Sleep(2 * time.Second)

	intervalStr := config.AppConfig.Serial.ScanInterval
	interval, err := time.ParseDuration(intervalStr)
	if err != nil || interval < time.Second {
		interval = 5 * time.Second
	}

	logger.Log.Infof("[%s] Starting polling loop with interval %v", w.PortName, interval)

	// Immediate poll
	w.poll()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stop:
			return
		case <-w.triggerChan:
			// Immediate poll triggered by URC
			w.poll()
		case <-ticker.C:
			w.poll()
		}
	}
}

func (w *ModemWorker) poll() {
	if w.getModem() == nil {
		return
	}
	if w.IsBusy() {
		return
	}
	if w.GetCallState().State != callStateIdle {
		return
	}
	w.checkSignal()
	// Only check SMS if this worker is the registered one for this ICCID
	if w.manager.IsRegisteredWorker(w.PortName, w.getModem().ICCID) {
		w.checkSMS()
	}
}

func (w *ModemWorker) checkOperator() {
	// +COPS: 0,0,"Chunghwa Telecom",7
	resp, err := w.ExecuteAT("AT+COPS?", 2*time.Second)
	if err != nil {
		logger.Log.Errorf("[%s] Failed COPS: %v", w.PortName, err)
		return
	}
	if strings.Contains(resp, "+COPS:") {
		// Basic parsing for string between quotes
		parts := strings.Split(resp, "\"")
		if len(parts) >= 3 {
			// parts[0] = +COPS: 0,0,
			// parts[1] = Chunghwa Telecom (Operator)
			// parts[2] = ,7
			w.modem.Operator = parts[1]
			// We delay saving to avoid aggressive DB writes, or just save
			// w.repo.Upsert(w.modem)
			// We are UPSERTING frequenly in signal check too.
		}
	}
}

func (w *ModemWorker) checkSignal() {
	// Registration drives whether operator should be shown.
	regCode := w.checkRegistration()
	if regCode == "1" || regCode == "5" {
		w.checkOperator()
	} else if regCode != "" {
		w.modem.Operator = ""
	}

	resp, err := w.ExecuteAT("AT+CSQ", 2*time.Second)
	if err != nil {
		logger.Log.Errorf("[%s] Failed CSQ: %v", w.PortName, err)
		return
	}
	// +CSQ: 20,99
	if strings.Contains(resp, "+CSQ:") {
		// Parse
		var rssi int
		// simple parsing logic
		parts := strings.Split(resp, ":")
		if len(parts) > 1 {
			vals := strings.Split(strings.TrimSpace(parts[1]), ",")
			if len(vals) > 0 {
				fmt.Sscanf(vals[0], "%d", &rssi)

				var signal int
				if rssi == 99 {
					signal = 0
				} else {
					// Convert 0-31 to 0-100%
					signal = int(float64(rssi) / 31.0 * 100.0)
				}

				w.modem.SignalStrength = signal
				w.modem.LastSeen = time.Now()
			}
		}
	}
}

func (w *ModemWorker) checkRegistration() string {
	resp, err := w.ExecuteAT("AT+CREG?", 2*time.Second)
	if err != nil {
		logger.Log.Errorf("[%s] Failed CREG: %v", w.PortName, err)
		return ""
	}

	code, text, err := parseCREGStatus(resp)
	if err != nil {
		logger.Log.Warnf("[%s] Failed to parse CREG response: %v", w.PortName, err)
		return ""
	}

	w.modem.Registration = text
	return code
}

func parseCREGStatus(resp string) (string, string, error) {
	body := strings.TrimSpace(parseID(resp, "+CREG:"))
	if body == "" {
		return "", "", fmt.Errorf("missing +CREG response")
	}

	parts := strings.Split(body, ",")
	if len(parts) == 0 {
		return "", "", fmt.Errorf("invalid +CREG response")
	}

	// +CREG: <n>,<stat>[,...]
	// Fallback for malformed payloads where only <stat> is present.
	stat := strings.TrimSpace(parts[0])
	if len(parts) >= 2 {
		stat = strings.TrimSpace(parts[1])
	}
	stat = strings.Trim(stat, `"`)

	switch stat {
	case "1":
		return stat, "Home Network", nil
	case "5":
		return stat, "Roaming", nil
	case "2":
		return stat, "Searching...", nil
	case "3":
		return stat, "Denied", nil
	case "4":
		return stat, "Unknown", nil
	case "0":
		return stat, "Not Registered", nil
	default:
		return stat, "Unknown", nil
	}
}

// readAndProcessSMS reads a specific SMS by index and processes it
func (w *ModemWorker) readAndProcessSMS(index int) {
	w.SetBusy(true)
	defer w.SetBusy(false)

	// Read specific message by index
	cmd := fmt.Sprintf("AT+CMGR=%d", index)
	resp, err := w.ExecuteAT(cmd, 5*time.Second)
	if err != nil {
		logger.Log.Errorf("[%s] Failed CMGR at index %d: %v", w.PortName, index, err)
		return
	}

	// Parse the response - format is +CMGR: <stat>,<oa>,<scts> then PDU
	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+CMGR:") {
			// Next line should be the PDU
			continue
		}
		if line == "" || line == "OK" {
			continue
		}
		// This line is the PDU
		if len(line) > 20 && isHexString(line) {
			w.processPDUWithIndex(line, index)
			break
		}
	}

	// Delete the message from SIM after processing
	delCmd := fmt.Sprintf("AT+CMGD=%d", index)
	if _, err := w.ExecuteAT(delCmd, 5*time.Second); err != nil {
		logger.Log.Warnf("[%s] Failed to delete SMS at index %d: %v", w.PortName, index, err)
	}
}

func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}

func (w *ModemWorker) checkSMS() {
	// PDU mode read all
	resp, err := w.ExecuteAT("AT+CMGL=4", 10*time.Second)
	if err != nil {
		logger.Log.Errorf("[%s] Failed CMGL: %v", w.PortName, err)
		return
	}
	if strings.TrimSpace(resp) == "OK" {
		return // No messages
	}

	lines := strings.Split(resp, "\n")
	var currentPDU string
	var currentIndex int
	var newMessages int

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "OK" {
			continue
		}
		if strings.HasPrefix(line, "+CMGL:") {
			// Extract index from +CMGL: <index>,<stat>,...
			parts := strings.SplitN(line, ",", 2)
			if len(parts) >= 1 {
				idxStr := strings.TrimPrefix(parts[0], "+CMGL:")
				idxStr = strings.TrimSpace(idxStr)
				if idx, err := strconv.Atoi(idxStr); err == nil {
					currentIndex = idx
				}
			}
		} else {
			// Likely PDU
			currentPDU = line
			w.processPDUWithIndex(currentPDU, currentIndex)
			newMessages++
		}
	}

	if newMessages > 0 {
		logger.Log.Infof("[%s] Processed %d new messages from SIM", w.PortName, newMessages)
	}

	// Delete all messages after reading to avoid filling memory
	if err := w.deleteReadMessages(lines); err != nil {
		logger.Log.Warnf("[%s] Failed to delete messages: %v", w.PortName, err)
	}
}

func (w *ModemWorker) processPDU(raw string) {
	w.processPDUWithIndex(raw, 0)
}

func (w *ModemWorker) processPDUWithIndex(raw string, simIndex int) {
	// Hex Decode
	b, err := hex.DecodeString(raw)
	if err != nil {
		logger.Log.Errorf("[%s] Failed to decode hex PDU: %v", w.PortName, err)
		return
	}

	// SMSC Address Handling
	// The first octet is the length of the SMSC field in octets
	if len(b) > 0 {
		smscLen := int(b[0])
		if len(b) > smscLen+1 {
			// Skip SMSC field (Len byte + Address bytes)
			b = b[smscLen+1:]
		}
	}

	// Use sms.Unmarshal (Default is AsMT - Mobile Terminated / Received)
	msg, err := sms.Unmarshal(b)
	if err != nil {
		logger.Log.Errorf("[%s] Failed to decode TPDU: %v", w.PortName, err)
	}

	var content string
	var sender string
	var timestamp time.Time = time.Now()

	if msg != nil {
		switch msg.SmsType() {
		case tpdu.SmsDeliver:
			sender = msg.OA.Number()
			timestamp = msg.SCTS.Time
		}

		// Use tpdu.DecodeUserData to correctly handle GSM7/UCS2 encoding
		alphabet, alphaErr := msg.DCS.Alphabet()
		var udContent []byte
		var decErr error

		if alphaErr != nil {
			decErr = alphaErr // Handle alpha error as decode error
		} else {
			udContent, decErr = tpdu.DecodeUserData(msg.UD, msg.UDH, alphabet)
		}

		if decErr == nil {
			content = string(udContent)
		} else {
			logger.Log.Warnf("[%s] Failed to decode UD: %v. DCS: %02X.", w.PortName, decErr, msg.DCS)
			// Fallback to simpler extraction or raw
			// If 7-bit, simply casting to string is wrong, but better than nothing for ASCII-like?
			// Actually better to show hex if it failed
			content = fmt.Sprintf("Decode Failed (DCS: 0x%02X)", msg.DCS)
		}

		// Final check
		if content == "" && len(msg.UD) > 0 {
			content = fmt.Sprintf("UD Hex: %X", msg.UD)
		}
	} else {
		// Decoding failed entirely previously
		content = fmt.Sprintf("Failed to decode PDU: %s", raw)
	}

	logger.Log.Infof("[%s] SMS From %s: %s", w.PortName, sender, content)

	sms := &model.SMS{
		ICCID:     w.getModem().ICCID,
		Phone:     sender,
		Content:   content,
		Timestamp: timestamp,
		Type:      "received",
		IsRead:    false,
		RawPDU:    raw,
		SimIndex:  simIndex,
		CreatedAt: time.Now(),
	}
	if sms.Timestamp.IsZero() {
		sms.Timestamp = time.Now()
	}

	err = w.smsRepo.Create(sms)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			logger.Log.Debugf("[%s] Duplicate SMS from %s, skipping webhook", w.PortName, sender)
			return
		}
		logger.Log.Errorf("[%s] Failed to save SMS: %v", w.PortName, err)
		return
	}

	// Trigger Webhook only for new (non-duplicate) messages
	w.webhookService.Dispatch(sms)
}

func (w *ModemWorker) deleteReadMessages(lines []string) error {
	// Instead of deleting all, we should iterate.
	// But AT+CMGD=1,4 deletes all.
	// The user asked to delete read messages.
	// Since we query AT+CMGL=4 (ALL), we can safely delete all AFTER processing.
	// However, to be safer, let's keep using Delete All for now as intended, but enable it always.
	// Or we can parse indices.
	// For "read after delete", CMGD=1,4 is fine if we processed everything.

	_, err := w.ExecuteAT("AT+CMGD=1,4", 5*time.Second)
	return err
}
