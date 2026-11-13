// This package provides support for XDG base directory specification and
// Filesystem Hierarchy Standard

package dirs

import (
	"os"
	"path/filepath"
)

// DataHome returns the XDG_DATA_HOME directory.
//
// XDG_DATA_HOME defines the base directory relative to which user-specific
// data files should be stored. If XDG_DATA_HOME is either not set or empty, a
// default equal to $HOME/.local/share should be used.
func DataHome() string {
	if s, ok := os.LookupEnv("XDG_DATA_HOME"); ok {
		return s
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share")
}

// DataDirs returns the XDG_DATA_DIRS directory.
//
// XDG_DATA_DIRS defines the preference-ordered set of base directories to
// search for data files in addition to the XDG_DATA_HOME base directory. The
// directories in XDG_DATA_DIRS should be separated with a colon ':'.
//
// If XDG_DATA_DIRS is either not set or empty, a value equal to
// /usr/local/share/:/usr/share/ should be used.
func DataDirs() []string {
	if s, ok := os.LookupEnv("XDG_DATA_DIRS"); ok {
		return filepath.SplitList(s)
	}
	return []string{"/usr/local/share", "/usr/share"}
}

// ConfigHome returns the XDG_CONFIG_HOME directory.
//
// XDG_CONFIG_HOME defines the base directory relative to which user-specific
// configuration files should be stored. If XDG_CONFIG_HOME is either not set
// or empty, a default equal to $HOME/.config should be used.
func ConfigHome() string {
	if s, ok := os.LookupEnv("XDG_CONFIG_HOME"); ok {
		return s
	}
	return filepath.Join(os.Getenv("HOME"), ".config")
}

// ConfigDirs returns the XDG_CONFIG_DIRS directory.
//
// XDG_CONFIG_DIRS defines the preference-ordered set of base directories to
// search for configuration files in addition to the XDG_CONFIG_HOME base
// directory. The directories in XDG_CONFIG_DIRS should be separated with a
// colon ':'.
//
// If XDG_CONFIG_DIRS is either not set or empty, a value equal to /etc/xdg
// should be used.
func ConfigDirs() []string {
	if s, ok := os.LookupEnv("XDG_CONFIG_DIRS"); ok {
		return filepath.SplitList(s)
	}
	return []string{"/etc/xdg"}
}

// CacheHome returns the XDG_CACHE_HOME directory.
//
// XDG_CACHE_HOME defines the base directory relative to which user-specific
// non-essential data files should be stored. If XDG_CACHE_HOME is either not
// set or empty, a default equal to $HOME/.cache should be used.
func CacheHome() string {
	if s, ok := os.LookupEnv("XDG_CACHE_HOME"); ok {
		return s
	}
	return filepath.Join(os.Getenv("HOME"), ".cache")
}

// StateHome returns the XDG_STATE_HOME directory.
//
// XDG_STATE_HOME defines the base directory relative to which user-specific
// state files should be stored (e.g. action history, logs, ...). If
// XDG_STATE_HOME is either not set or empty, a default equal to
// $HOME/.local/state should be used.
func StateHome() string {
	if s, ok := os.LookupEnv("XDG_STATE_HOME"); ok {
		return s
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "state")
}

// BinDirHome returns the directory for user-specific executables
// $HOME/.local/bin.
//
// Distributions should ensure this directory shows up in the UNIX $PATH
// environment variable, at an appropriate place.
func BinDirHome() string {
	return filepath.Join(os.Getenv("HOME"), ".local", "bin")
}

// RuntimeDir returns the XDG_RUNTIME_DIR directory.
//
// XDG_RUNTIME_DIR defines the base directory relative to which user-specific
// non-essential runtime files and other file objects (such as sockets, named
// pipes, ...) should be stored. The directory MUST be owned by the user, and
// he MUST be the only one having read and write access to it. Its Unix access
// mode MUST be 0700.
//
// The lifetime of the directory MUST be bound to the user being logged in. It
// MUST be created when the user first logs in and if the user fully logs out
// the directory MUST be removed. If the user logs in more than once he should
// get pointed to the same directory, and it is mandatory that the directory
// continues to exist from his first login to his last logout on the system,
// and not removed in between. Files in the directory MUST not survive reboot
// or a full logout/login cycle.
//
// The directory MUST be on a local file system and not shared with any other
// system. The directory MUST by fully-featured by the standards of the
// operating system. More specifically, on Unix-like operating systems AF_UNIX
// sockets, symbolic links, hard links, proper permissions, file locking,
// sparse files, memory mapping, file change notifications, a reliable hard
// link count must be supported, and no restrictions on the file name character
// set should be imposed. Files in this directory MAY be subjected to periodic
// clean-up. To ensure that your files are not removed, they should have their
// access time timestamp modified at least once every 6 hours of monotonic time
// or the 'sticky' bit should be set on the file.
//
// If XDG_RUNTIME_DIR is not set applications should fall back to a
// replacement directory with similar capabilities and print a warning message.
// Applications should use this directory for communication and synchronization
// purposes and should not place larger files in it, since it might reside in
// runtime memory and cannot necessarily be swapped out to disk.
func RuntimeDir() string {
	return os.Getenv("XDG_RUNTIME_DIR")
}
