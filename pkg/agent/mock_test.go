package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockRecorder(t *testing.T) {
	recorder := NewMockRecorder()

	t.Run("Start", func(t *testing.T) {
		session, err := recorder.Start(RecordingTypeTroubleshoot, "session-1", "host", "user")
		require.NoError(t, err)
		assert.Equal(t, RecordingTypeTroubleshoot, session.Type)
		assert.Equal(t, "session-1", session.SessionID)
	})

	t.Run("RecordInput", func(t *testing.T) {
		err := recorder.RecordInput("session-1", "ls -la")
		require.NoError(t, err)

		status := recorder.GetStatus()
		assert.Equal(t, 1, status.CommandCount)
	})

	t.Run("Stop", func(t *testing.T) {
		session, err := recorder.Stop()
		require.NoError(t, err)
		assert.Len(t, session.Commands, 1)
		assert.Equal(t, "ls -la", session.Commands[0].Content)

		status := recorder.GetStatus()
		assert.False(t, status.IsRecording)
	})

	t.Run("DoubleStart", func(t *testing.T) {
		_, err := recorder.Start(RecordingTypeScript, "session-2", "host", "user")
		require.NoError(t, err)

		_, err = recorder.Start(RecordingTypeScript, "session-3", "host", "user")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already recording")

		_, _ = recorder.Stop()
	})
}

func TestMockTroubleshotter(t *testing.T) {
	ts := NewMockTroubleshotter()

	t.Run("StartCase", func(t *testing.T) {
		caseData, err := ts.StartCase("test problem", []string{"ctx1"}, "session-1", "host", "user")
		require.NoError(t, err)
		assert.Equal(t, "test problem", caseData.Problem)
		assert.Equal(t, RecordingTypeTroubleshoot, caseData.Type)
	})

	t.Run("GetStatus", func(t *testing.T) {
		status := ts.GetStatus()
		assert.True(t, status.IsActive)
		assert.Equal(t, "test problem", status.Problem)
	})

	t.Run("GetCurrentCase", func(t *testing.T) {
		caseData := ts.GetCurrentCase()
		require.NotNil(t, caseData)
		assert.Equal(t, "test problem", caseData.Problem)
	})

	t.Run("StopCase", func(t *testing.T) {
		caseData, err := ts.StopCase("root cause", "conclusion")
		require.NoError(t, err)
		assert.Equal(t, "root cause", caseData.RootCause)
		assert.Equal(t, "conclusion", caseData.Conclusion)

		status := ts.GetStatus()
		assert.False(t, status.IsActive)
	})

	t.Run("ListCases", func(t *testing.T) {
		cases, err := ts.ListCases()
		require.NoError(t, err)
		assert.Len(t, cases, 1)
	})

	t.Run("LoadCase", func(t *testing.T) {
		caseData := ts.GetCurrentCase()
		require.Nil(t, caseData)

		// 从列表中获取
		cases, _ := ts.ListCases()
		require.Len(t, cases, 1)

		loaded, err := ts.LoadCase(cases[0].ID)
		require.NoError(t, err)
		assert.Equal(t, "test problem", loaded.Problem)
	})

	t.Run("DeleteCase", func(t *testing.T) {
		cases, _ := ts.ListCases()
		require.Len(t, cases, 1)

		err := ts.DeleteCase(cases[0].ID)
		require.NoError(t, err)

		cases, _ = ts.ListCases()
		assert.Empty(t, cases)
	})

	t.Run("GenerateDocument", func(t *testing.T) {
		// 先创建一个案例
		caseData, _ := ts.StartCase("new problem", nil, "s2", "h", "u")
		_, _ = ts.StopCase("rc", "concl")

		doc, err := ts.GenerateDocument(caseData.ID)
		require.NoError(t, err)
		assert.Contains(t, doc, "new problem")
	})
}

func TestMockAgentService(t *testing.T) {
	service := NewMockAgentService()

	t.Run("Start", func(t *testing.T) {
		err := service.Start(nil)
		require.NoError(t, err)
		assert.True(t, service.GetStatus().Running)
	})

	t.Run("ProcessUserInput", func(t *testing.T) {
		resp, err := service.ProcessUserInput(nil, "session-1", "help")
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Contains(t, resp.Message, "help")
	})

	t.Run("Stop", func(t *testing.T) {
		err := service.Stop()
		require.NoError(t, err)
		assert.False(t, service.GetStatus().Running)
	})
}
