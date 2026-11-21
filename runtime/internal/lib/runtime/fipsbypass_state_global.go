//go:build go1.26 && !llgo

package runtime

// Збірка без llgo не має TLS-локальності llgo, тож тут звичайна глобальна змінна.
var fipsBypassDepth uint32
