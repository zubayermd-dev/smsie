package api

import (
	"encoding/hex"
	"fmt"
	"github.com/zubayermd-dev/ivy/internal/worker"
	"github.com/zubayermd-dev/ivy/pkg/logger"
	"github.com/warthog618/sms/encoding/tpdu"
	"log"
	"net/http"
	"strconv"

	"github.com/warthog618/sms"

	"github.com/gin-gonic/gin"
	"github.com/zubayermd-dev/ivy/internal/model"
	"gorm.io/gorm"
)

type SMSHandler struct {
	db *gorm.DB
	wm *worker.Manager
}

func NewSMSHandler(db *gorm.DB, wm *worker.Manager) *SMSHandler {
	return &SMSHandler{db: db, wm: wm}
}

func (h *SMSHandler) ListSMS(c *gin.Context) {
	actor, ok := getActor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if actor.APIKey != nil && !actor.APIKey.CanViewSMS {
		c.JSON(http.StatusForbidden, gin.H{"error": "API key permission denied"})
		return
	}

	limitStr := c.DefaultQuery("limit", "20")
	pageStr := c.DefaultQuery("page", "1")

	var limit int
	var page int

	// Helper logic to parse int not included here, assuming basic strconv or just string input if possible?
	// No, better implement strict parsing.
	// Since imports are limited, let's use safe unchecked casts or add strconv.
	// Adding strconv to imports below.

	// Simplified parsing with limit cap
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	} else {
		limit = 20
	}
	// Cap limit to prevent DoS via large result sets
	if limit > 500 {
		limit = 500
	}
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	} else {
		page = 1
	}

	iccid := c.Query("iccid")
	allowedForView, err := allowedICCIDsForPermission(h.db, actor.User, PermViewSMS)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "permission check failed"})
		return
	}
	isAdmin := actor.User != nil && actor.User.Role == "admin"

	query := h.db.Model(&model.SMS{}) // Start with model to allow counting

	if iccid != "" {
		if !enforceICCIDPermission(c, h.db, iccid, PermViewSMS) {
			return
		}
		query = query.Where("iccid = ?", iccid)
	} else {
		if !isAdmin {
			if len(allowedForView) == 0 {
				query = query.Where("1 = 0")
			} else if hasWildcardICCID(allowedForView) {
				// wildcard: do not constrain iccid
			} else {
				query = query.Where("iccid IN ?", allowedForView)
			}
		}
	}

	var total int64
	query.Count(&total)

	var smsList []model.SMS
	offset := (page - 1) * limit
	if err := query.Order("timestamp desc").Limit(limit).Offset(offset).Find(&smsList).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for i, s := range smsList {
		if s.Content == "" && s.Phone == "" {
			d, _ := hex.DecodeString(s.RawPDU)
			// SMSC Address Handling
			// The first octet is the length of the SMSC field in octets
			if len(d) > 0 {
				smscLen := int(d[0])
				if len(d) > smscLen+1 {
					// Skip SMSC field (Len byte + Address bytes)
					d = d[smscLen+1:]
				}
			}
			msg, err := sms.Unmarshal(d)
			content := ""
			if err != nil {
				logger.Log.Warnf("Failed to unmarshal sms pdu: %v", err)
				continue
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
				logger.Log.Warnf("[] Failed to decode UD: %v. DCS: %02X.", decErr, msg.DCS)
				// Fallback to simpler extraction or raw
				// If 7-bit, simply casting to string is wrong, but better than nothing for ASCII-like?
				// Actually better to show hex if it failed
				content = fmt.Sprintf("Decode Failed (DCS: 0x%02X)", msg.DCS)
			}

			// Final check
			if content == "" && len(msg.UD) > 0 {
				content = fmt.Sprintf("UD Hex: %X", msg.UD)
			}
			log.Println(content)
			smsList[i].Content = content
			smsList[i].Phone = msg.OA.Number()
			h.db.Updates(&smsList[i])
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  smsList,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (h *SMSHandler) DeleteSMS(c *gin.Context) {
	actor, ok := getActor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid SMS ID"})
		return
	}

	var smsObj model.SMS
	if err := h.db.First(&smsObj, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "SMS not found"})
		return
	}

	// Check permission
	if !enforceICCIDPermission(c, h.db, smsObj.ICCID, PermViewSMS) {
		return
	}

	// Only admin or the user who owns the API key can delete
	isAdmin := actor.User != nil && actor.User.Role == "admin"
	if !isAdmin && actor.APIKey == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Delete from SIM card if index is available
	if smsObj.SimIndex > 0 {
		w := h.wm.GetWorkerByICCID(smsObj.ICCID)
		if w != nil {
			if err := w.DeleteSMSFromSIM(smsObj.SimIndex); err != nil {
				logger.Log.Warnf("Failed to delete SMS from SIM: %v", err)
			}
		}
	}

	if err := h.db.Delete(&smsObj).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete SMS"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *SMSHandler) DeleteByPhone(c *gin.Context) {
	actor, ok := getActor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	phone := c.Query("phone")
	if phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone query required"})
		return
	}

	isAdmin := actor.User != nil && actor.User.Role == "admin"
	allowedForView, err := allowedICCIDsForPermission(h.db, actor.User, PermViewSMS)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "permission check failed"})
		return
	}

	query := h.db.Where("phone = ?", phone)
	if !isAdmin {
		if len(allowedForView) == 0 {
			query = query.Where("1 = 0")
		} else if !hasWildcardICCID(allowedForView) {
			query = query.Where("iccid IN ?", allowedForView)
		}
	}

	result := query.Delete(&model.SMS{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete messages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "count": result.RowsAffected})
}

func (h *SMSHandler) MarkAsRead(c *gin.Context) {
	actor, ok := getActor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	iccid := c.Query("iccid")
	phone := c.Query("phone")

	isAdmin := actor.User != nil && actor.User.Role == "admin"
	allowedForView, err := allowedICCIDsForPermission(h.db, actor.User, PermViewSMS)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "permission check failed"})
		return
	}

	// Build base query with permission filter
	query := h.db.Model(&model.SMS{}).Where("is_read = ?", false)
	if !isAdmin {
		if len(allowedForView) == 0 {
			query = query.Where("1 = 0")
		} else if !hasWildcardICCID(allowedForView) {
			query = query.Where("iccid IN ?", allowedForView)
		}
	}

	if iccid != "" {
		if !enforceICCIDPermission(c, h.db, iccid, PermViewSMS) {
			return
		}
		query = query.Where("iccid = ?", iccid)
	}
	if phone != "" {
		query = query.Where("phone = ?", phone)
	}

	query.Update("is_read", true)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
