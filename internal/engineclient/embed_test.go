package engineclient

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnginePath_ExtractsAndReturnsExecutable(t *testing.T) {
	path, err := EnginePath()
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.NotZero(t, info.Mode()&0o111, "binary should be executable")
}

func TestEnginePath_IdempotentOnSecondCall(t *testing.T) {
	path1, err := EnginePath()
	require.NoError(t, err)

	path2, err := EnginePath()
	require.NoError(t, err)

	assert.Equal(t, path1, path2)
}
