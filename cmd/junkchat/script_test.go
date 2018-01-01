package junkchat

import (
	"testing"

	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseScript(t *testing.T) {
	got, err := ParseScript("")
	require.Error(t, err)

	got, err = ParseScript("T1s")
	require.NoError(t, err)
	assert.Equal(t, []Action{{Duration: time.Second}}, got)

	got, err = ParseScript("W1M")
	require.NoError(t, err)
	assert.Equal(t, []Action{{Write: 1024 * 1024}}, got)

	got, err = ParseScript("R1,T5s")
	require.NoError(t, err)
	assert.Equal(t, []Action{{Read: 1, Duration: time.Second * 5}}, got)

	got, err = ParseScript("R1. T1s")
	require.NoError(t, err)
	assert.Equal(t, []Action{{Read: 1}, {Duration: time.Second}}, got)
}
