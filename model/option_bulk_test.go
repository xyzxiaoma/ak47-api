package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionsBulkRollsBackPricingOptionsAsOneTransaction(t *testing.T) {
	db := useFrontendOptionMigrationDB(t)
	require.NoError(t, db.Migrator().DropTable(&Option{}))
	require.NoError(t, db.Exec(`CREATE TABLE options (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL CHECK (value <> 'reject')
	)`).Error)

	originalModelGroupRatios := ratio_setting.ModelGroupRatio2JSONString()
	err := UpdateOptionsBulk(map[string]string{
		"ModelGroupRatio": `{"deepseek-v4-pro":0.3}`,
		"ForcedFailure":   "reject",
	})
	require.Error(t, err)
	require.Equal(t, originalModelGroupRatios, ratio_setting.ModelGroupRatio2JSONString())

	for _, key := range []string{"ModelGroupRatio", "ForcedFailure"} {
		var option Option
		require.ErrorIs(t, db.Where("key = ?", key).First(&option).Error, gorm.ErrRecordNotFound)
	}
}
