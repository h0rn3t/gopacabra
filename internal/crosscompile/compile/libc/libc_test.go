package libc

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGetPicolibcConfig_LibConfig(t *testing.T) {
	config := GetPicolibcConfig()

	// Test basic configuration fields
	expectedName := "picolibc"
	if config.Name != expectedName {
		t.Errorf("Expected Name '%s', got '%s'", expectedName, config.Name)
	}

	expectedVersion := "v0.1.0"
	if config.Version != expectedVersion {
		t.Errorf("Expected Version '%s', got '%s'", expectedVersion, config.Version)
	}

	expectedUrl := "https://github.com/goplus/picolibc/archive/refs/heads/main.zip"
	if config.Url != expectedUrl {
		t.Errorf("Expected Url '%s', got '%s'", expectedUrl, config.Url)
	}

	expectedArchiveSrcDir := "picolibc-main"
	if config.ResourceSubDir != expectedArchiveSrcDir {
		t.Errorf("Expected ResourceSubDir '%s', got '%s'", expectedArchiveSrcDir, config.ResourceSubDir)
	}

	// Test String() method
	expectedString := "picolibc-v0.1.0"
	if config.String() != expectedString {
		t.Errorf("Expected String() '%s', got '%s'", expectedString, config.String())
	}
}

func TestGetPicolibcCompileConfig(t *testing.T) {
	baseDir := filepath.FromSlash("/test/base")
	target := "test-target"

	config := GetPicolibcCompileConfig(baseDir, target)

	// Test LibcCFlags
	if len(config.ExportCFlags) != 2 {
		t.Errorf("Expected 2 LibcCFlags, got %d", len(config.ExportCFlags))
	} else {
		expected := "-I" + baseDir
		if config.ExportCFlags[0] != expected {
			t.Errorf("Expected LibcCFlags[0] to be '%s', got '%s'", expected, config.ExportCFlags[0])
		}

		expected = "-isystem" + filepath.Join(baseDir, "newlib", "libc", "include")
		if config.ExportCFlags[1] != expected {
			t.Errorf("Expected LibcCFlags[1] to be '%s', got '%s'", expected, config.ExportCFlags[1])
		}
	}

	// Test Groups configuration
	if len(config.Groups) != 1 {
		t.Errorf("Expected 1 group, got %d", len(config.Groups))
	} else {
		group := config.Groups[0]

		// Test output file name
		expectedOutput := "libc-" + target + ".a"
		if group.OutputFileName != expectedOutput {
			t.Errorf("Expected OutputFileName '%s', got '%s'", expectedOutput, group.OutputFileName)
		}

		// Test files list
		if len(group.Files) == 0 {
			t.Error("Expected non-empty files list")
		} else {
			// Check a few sample files
			sampleFiles := []string{
				filepath.Join(baseDir, "newlib", "libc", "string", "bcmp.c"),
				filepath.Join(baseDir, "newlib", "libc", "string", "memcpy.c"),
				filepath.Join(baseDir, "newlib", "libc", "string", "strlen.c"),
				filepath.Join(baseDir, "newlib", "libc", "stdlib", "nano-malloc.c"),
				filepath.Join(baseDir, "newlib", "libc", "tinystdio", "printf.c"),
			}

			for _, sample := range sampleFiles {
				found := false
				for _, file := range group.Files {
					if file == sample {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected file '%s' not found in group files", sample)
				}
			}
		}

		// Test CFlags
		expectedCFlags := []string{
			"-D_COMPILING_NEWLIB",
			"-D_HAVE_ALIAS_ATTRIBUTE",
			"-DTINY_STDIO",
			"-DPOSIX_IO",
			"-DFORMAT_DEFAULT_INTEGER",
			"-D_IEEE_LIBM",
			"-D__OBSOLETE_MATH_FLOAT=1",
			"-D__OBSOLETE_MATH_DOUBLE=0",
			"-D_WANT_IO_C99_FORMATS",
			"-nostdlib",
			"-I" + baseDir,
			"-isystem" + filepath.Join(baseDir, "newlib", "libc", "include"),
			"-I" + filepath.Join(baseDir, "newlib", "libm", "common"),
			"-I" + filepath.Join(baseDir, "newlib", "libc", "locale"),
			"-I" + filepath.Join(baseDir, "newlib", "libc", "tinystdio"),
		}

		if len(group.CFlags) != len(expectedCFlags) {
			t.Errorf("Expected %d CFlags, got %d", len(expectedCFlags), len(group.CFlags))
		} else {
			for i, expected := range expectedCFlags {
				if group.CFlags[i] != expected {
					t.Errorf("CFlags[%d] mismatch. Expected '%s', got '%s'", i, expected, group.CFlags[i])
				}
			}
		}

		// Test LDFlags and CCFlags
		if len(group.LDFlags) == 0 {
			t.Error("Expected non-empty LDFlags")
		}
		if len(group.CCFlags) == 0 {
			t.Error("Expected non-empty CCFlags")
		}
	}
}

func TestGetPicolibcConfig_EdgeCases(t *testing.T) {
	t.Run("EmptyBaseDir", func(t *testing.T) {
		config := GetPicolibcCompileConfig("", "test-target")

		// Check that paths are constructed correctly even with empty baseDir
		expected := "-I"
		if config.ExportCFlags[0] != expected {
			t.Errorf("Expected LibcCFlags[0] to be '%s', got '%s'", expected, config.ExportCFlags[0])
		}

		expected = "-isystem" + filepath.Join("", "newlib", "libc", "include")
		if config.ExportCFlags[1] != expected {
			t.Errorf("Expected LibcCFlags[1] to be '%s', got '%s'", expected, config.ExportCFlags[1])
		}
	})

	t.Run("EmptyTarget", func(t *testing.T) {
		config := GetPicolibcCompileConfig(filepath.FromSlash("/test/base"), "")

		// Check output file name formatting
		expectedOutput := "libc-.a"
		if config.Groups[0].OutputFileName != expectedOutput {
			t.Errorf("Expected OutputFileName '%s', got '%s'", expectedOutput, config.Groups[0].OutputFileName)
		}
	})
}

func TestPicolibcFileStructure(t *testing.T) {
	baseDir := filepath.FromSlash("/test/base")
	target := "test-target"

	config := GetPicolibcCompileConfig(baseDir, target)
	group := config.Groups[0]

	// Test that all files have .c extension (no assembly files in picolibc config)
	for _, file := range group.Files {
		if !strings.HasSuffix(file, ".c") {
			t.Errorf("File '%s' does not have .c extension", file)
		}
	}

	// Test that files are from expected directories
	stringFiles := 0
	stdlibFiles := 0
	tinystdioFiles := 0

	for _, file := range group.Files {
		if strings.Contains(file, string(filepath.Separator)+"string"+string(filepath.Separator)) {
			stringFiles++
		} else if strings.Contains(file, string(filepath.Separator)+"stdlib"+string(filepath.Separator)) {
			stdlibFiles++
		} else if strings.Contains(file, string(filepath.Separator)+"tinystdio"+string(filepath.Separator)) {
			tinystdioFiles++
		}
	}

	if stringFiles < 50 {
		t.Errorf("Expected at least 50 string files, got %d", stringFiles)
	}
	if stdlibFiles < 5 {
		t.Errorf("Expected at least 5 stdlib files, got %d", stdlibFiles)
	}
	if tinystdioFiles < 3 {
		t.Errorf("Expected at least 3 tinystdio files, got %d", tinystdioFiles)
	}

	// Test that all files have correct base directory
	for _, file := range group.Files {
		if !strings.HasPrefix(file, baseDir) {
			t.Errorf("File '%s' does not have expected base directory '%s'", file, baseDir)
		}
	}
}

func TestPicolibcCompilerFlags(t *testing.T) {
	baseDir := filepath.FromSlash("/test/base")
	target := "test-target"

	config := GetPicolibcCompileConfig(baseDir, target)
	group := config.Groups[0]

	// Test that required preprocessor definitions are present
	requiredDefines := []string{
		"-D_COMPILING_NEWLIB",
		"-D_HAVE_ALIAS_ATTRIBUTE",
		"-DTINY_STDIO",
		"-DPOSIX_IO",
		"-DFORMAT_DEFAULT_INTEGER",
		"-D_IEEE_LIBM",
		"-D_WANT_IO_C99_FORMATS",
	}

	for _, define := range requiredDefines {
		found := false
		for _, flag := range group.CFlags {
			if flag == define {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Required define '%s' not found in CFlags", define)
		}
	}

	// Test that required include paths are present
	requiredIncludes := []string{
		"-I" + baseDir,
		"-isystem" + filepath.Join(baseDir, "newlib", "libc", "include"),
		"-I" + filepath.Join(baseDir, "newlib", "libm", "common"),
		"-I" + filepath.Join(baseDir, "newlib", "libc", "locale"),
		"-I" + filepath.Join(baseDir, "newlib", "libc", "tinystdio"),
	}

	for _, include := range requiredIncludes {
		found := false
		for _, flag := range group.CFlags {
			if flag == include {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Required include '%s' not found in CFlags", include)
		}
	}

	// Test that nostdlib is present
	found := false
	for _, flag := range group.CFlags {
		if flag == "-nostdlib" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Required flag '-nostdlib' not found in CFlags")
	}
}
