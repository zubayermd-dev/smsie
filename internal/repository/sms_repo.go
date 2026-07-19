package repository

import (
	"errors"
	"time"

	"github.com/zubayermd-dev/ivy/internal/model"
	"gorm.io/gorm"
)

var ErrDuplicate = errors.New("duplicate message")

type SMSRepository struct {
	db *gorm.DB
}

func NewSMSRepository(db *gorm.DB) *SMSRepository {
	return &SMSRepository{db: db}
}

func (r *SMSRepository) Create(sms *model.SMS) error {
	// Deduplication in a transaction to prevent race conditions
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Check for duplicate within 10 minutes (carrier may resend same SMS)
		since := time.Now().Add(-10 * time.Minute)
		var count int64
		tx.Model(&model.SMS{}).
			Where("phone = ? AND content = ? AND created_at > ?",
				sms.Phone, sms.Content, since).
			Count(&count)
		if count > 0 {
			return ErrDuplicate
		}
		return tx.Create(sms).Error
	})
}

func (r *SMSRepository) FindByICCID(iccid string) ([]model.SMS, error) {
	var smsList []model.SMS
	err := r.db.Where("iccid = ?", iccid).Order("timestamp desc").Find(&smsList).Error
	return smsList, err
}
