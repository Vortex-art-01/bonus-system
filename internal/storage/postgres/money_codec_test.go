package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vortex-art-01/bonus-system/internal/model"
)

func TestMoneyPgxRoundTrip(t *testing.T) {
	m := pgtype.NewMap()

	for _, format := range []int16{pgtype.BinaryFormatCode, pgtype.TextFormatCode} {
		for _, want := range []model.Money{0, 1, 50050, 72998, -1, -72998} {
			buf, err := m.Encode(pgtype.Int8OID, format, want, nil)
			require.NoError(t, err, "encode %s", want)

			var got model.Money
			require.NoError(t, m.Scan(pgtype.Int8OID, format, buf, &got))
			assert.Equal(t, want, got)

			var ptr *model.Money
			require.NoError(t, m.Scan(pgtype.Int8OID, format, buf, &ptr))
			require.NotNil(t, ptr)
			assert.Equal(t, want, *ptr)
		}
	}
}

func TestMoneyPgxNull(t *testing.T) {
	m := pgtype.NewMap()

	ptr := new(model.Money)
	require.NoError(t, m.Scan(pgtype.Int8OID, pgtype.BinaryFormatCode, nil, &ptr))
	assert.Nil(t, ptr, "a NULL accrual must scan into a nil *model.Money")

	buf, err := m.Encode(pgtype.Int8OID, pgtype.BinaryFormatCode, (*model.Money)(nil), nil)
	require.NoError(t, err)
	assert.Nil(t, buf, "a nil *model.Money must encode as NULL")
}
