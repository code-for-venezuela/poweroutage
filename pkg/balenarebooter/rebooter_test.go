package balenarerebooter

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteTimestampToFile(t *testing.T) {
	// Setup
	tempFile, err := ioutil.TempFile("", "timestamp")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())

	rebooter := New(0, 0, tempFile.Name())

	// Test timestamp writing
	testTime := time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC)
	err = rebooter.WriteTimestampToFile(testTime)
	require.NoError(t, err)

	// Verify file content
	actualTime, err := rebooter.GetLastRestartTimestamp()
	require.NoError(t, err)
	assert.Equal(t, testTime.UTC(), actualTime.UTC())
}

func TestGetLastRestartTimestamp(t *testing.T) {
	// Setup
	tempFile, err := ioutil.TempFile("", "timestamp")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())

	expectedTime := time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC)

	rebooter := New(0, 0, tempFile.Name())
	err = rebooter.WriteTimestampToFile(expectedTime)
	require.NoError(t, err)

	// Test getting the last restart timestamp
	actualTime, err := rebooter.GetLastRestartTimestamp()
	require.NoError(t, err)
	assert.Equal(t, expectedTime.Unix(), actualTime.Unix())
}

func TestShouldRestart(t *testing.T) {
	// Setup
	tempFile, err := ioutil.TempFile("", "timestamp")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())

	// Write a timestamp that is older than 24 hours
	oldTime := time.Now().Add(-25 * time.Hour)
	ioutil.WriteFile(tempFile.Name(), []byte(strconv.FormatInt(oldTime.Unix(), 10)), 0644)

	rebooter := New(1*time.Hour, 24*time.Hour, tempFile.Name())

	// Test shouldRestart
	shouldRestart := rebooter.shouldRestart()
	assert.True(t, shouldRestart)

	// Verify file content has been updated to a more recent timestamp
	content, err := ioutil.ReadFile(tempFile.Name())
	require.NoError(t, err)
	newTimestamp, err := strconv.ParseInt(string(content), 10, 64)
	require.NoError(t, err)
	assert.True(t, newTimestamp > oldTime.Unix())
}

// Additional tests can be written for Start, Stop, and restartApp methods.
// These would likely require more sophisticated mocking or integration testing setup,
// especially for testing HTTP requests and asynchronous behavior.
