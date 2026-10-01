package uiact

import "testing"

func TestAnAppIsFoundByItsBundleWhenItsProcessHasAnotherName(t *testing.T) {
	listing := `  501 /sbin/launchd
53489 /Applications/Antigravity IDE.app/Contents/MacOS/Electron
53493 /Applications/Antigravity IDE.app/Contents/Frameworks/Antigravity IDE Helper.app/Contents/MacOS/Antigravity IDE Helper
 2144 /Applications/Notion.app/Contents/MacOS/Notion
`
	if pid, ok := bundlePid(listing, "antigravity ide"); !ok || pid != 53489 {
		t.Errorf("pid = %d (found=%v), want the main process and not a helper", pid, ok)
	}
	if pid, ok := bundlePid(listing, "Notion"); !ok || pid != 2144 {
		t.Errorf("pid = %d (found=%v)", pid, ok)
	}
	if _, ok := bundlePid(listing, "Slack"); ok {
		t.Error("found an application that is not running")
	}
}
