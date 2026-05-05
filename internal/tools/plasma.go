package tools

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

var dbusConn *dbus.Conn

func init() {
	// Connect to session D-Bus once; keep nil if unavailable.
	var err error
	dbusConn, err = dbus.ConnectSessionBus()
	if err != nil {
		// No D-Bus; methods will report unavailability.
		dbusConn = nil
	}
}

// PlasmaStatus reports availability and supported calls.
func PlasmaStatus() map[string]any {
	if dbusConn == nil {
		return map[string]any{"available": false, "reason": "D-Bus session not available"}
	}
	return map[string]any{
		"available": true,
		"service":   "org.kde.Plasma",
		"methods":   []string{"SetTheme", "SetWallpaper", "GetPanelConfig", "SetWidgetConfig"},
	}
}

// SetTheme changes the Plasma theme.
// theme: theme name (e.g. "Breeze", "Oxygen", custom theme name).
func SetTheme(theme string) error {
	if dbusConn == nil {
		return fmt.Errorf("D-Bus session not available")
	}

	// D-Bus: org.kde.Plasma.DesktopShell.SetTheme
	obj := dbusConn.Object("org.kde.Plasma", "/org/kde/Plasma/DesktopShell")
	call := obj.Call("org.kde.Plasma.DesktopShell.SetTheme", 0, theme)
	if call.Err != nil {
		return fmt.Errorf("failed to set Plasma theme: %w", call.Err)
	}
	return nil
}

// SetWallpaper changes the desktop wallpaper.
// imagePath: absolute path to the image file.
func SetWallpaper(imagePath string) error {
	if dbusConn == nil {
		return fmt.Errorf("D-Bus session not available")
	}

	// D-Bus: org.kde.Plasma.DesktopShell.SetWallpaper
	obj := dbusConn.Object("org.kde.Plasma", "/org/kde/Plasma/DesktopShell")
	call := obj.Call("org.kde.Plasma.DesktopShell.SetWallpaper", 0, imagePath)
	if call.Err != nil {
		return fmt.Errorf("failed to set Plasma wallpaper: %w", call.Err)
	}
	return nil
}

// GetPanelConfig fetches current panel config.
// Returns panel name, position, height, and widgets.
func GetPanelConfig() (map[string]any, error) {
	if dbusConn == nil {
		return nil, fmt.Errorf("D-Bus session not available")
	}

	// D-Bus: org.kde.Plasma.GetPanelConfig
	obj := dbusConn.Object("org.kde.Plasma", "/org/kde/Plasma")
	call := obj.Call("org.kde.Plasma.GetPanelConfig", 0)
	if call.Err != nil {
		return nil, fmt.Errorf("failed to get panel config: %w", call.Err)
	}

	var config map[string]any
	if err := call.Store(&config); err != nil {
		return nil, fmt.Errorf("failed to parse panel config: %w", err)
	}
	return config, nil
}

// SetWidgetConfig updates a specific widget config.
// widgetName: widget name (e.g. "taskmanager", "systemtray", "clock").
// config: key-value config map.
func SetWidgetConfig(widgetName string, config map[string]string) error {
	if dbusConn == nil {
		return fmt.Errorf("D-Bus session not available")
	}

	// D-Bus: org.kde.Plasma.Widgets.SetConfig
	path := dbus.ObjectPath(fmt.Sprintf("/org/kde/Plasma/Widgets/%s", widgetName))
	obj := dbusConn.Object("org.kde.Plasma", path)
	call := obj.Call("org.kde.Plasma.Widgets.SetConfig", 0, config)
	if call.Err != nil {
		return fmt.Errorf("failed to set widget config for %s: %w", widgetName, call.Err)
	}
	return nil
}

// ListPanelWidgets returns active panel widgets.
func ListPanelWidgets() ([]map[string]any, error) {
	if dbusConn == nil {
		return nil, fmt.Errorf("D-Bus session not available")
	}

	obj := dbusConn.Object("org.kde.Plasma", "/org/kde/Plasma")
	call := obj.Call("org.kde.Plasma.ListWidgets", 0)
	if call.Err != nil {
		return nil, fmt.Errorf("failed to list widgets: %w", call.Err)
	}

	var widgets []map[string]any
	if err := call.Store(&widgets); err != nil {
		return nil, fmt.Errorf("failed to parse widgets list: %w", err)
	}
	return widgets, nil
}
