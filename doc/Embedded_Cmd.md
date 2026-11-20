# LLGo Embedded Development Command Line Options

## Flags

- `-o <file>` - Specify output file name
- `-target <platform>` - Specify target platform for cross-compilation
- `-obin` - Generate binary format output (requires `-target`)
- `-ohex` - Generate Intel HEX format output (requires `-target`)
- `-oimg` - Generate firmware image format output (requires `-target`)
- `-ouf2` - Generate UF2 format output (requires `-target`)
- `-ozip` - Generate ZIP/DFU format output (requires `-target`)
- `-emulator` - Run using emulator (auto-detects required format)
- `-port <port>` - Target port for flashing, testing, or monitoring
- `-baudrate <rate>` - Baudrate for serial communication (default: 115200)

## Commands

### llgo build
Compile program to output file.
- No `-target`: Native executable
- With `-target`: ELF executable (and additional formats if specified with `-obin`, `-ohex`, etc.)

### llgo run
Compile and run program.
- No `-target`: Run locally
- With `-target`: Run on device or emulator (equivalent to `install` + `monitor`)

### llgo test
Compile and run tests.
- No `-target`: Run tests locally
- With `-target`: Run tests on device or emulator
- Supports `-emulator` and `-port` flags

### llgo install
Install program or flash to device.
- No `-target`: Install to `$GOPATH/bin`
- With `-target`: Flash to device (use `-port` to specify port)

### llgo monitor
Monitor serial output from embedded device.
- `-port <device>`: Serial port device (e.g., `/dev/ttyUSB0`)
- `-target <platform>`: Auto-detect port from target configuration
- `-baudrate <rate>`: Communication speed (default: 115200)
- `[executable]`: Optional ELF file for debug info (panic address resolution)

Features:
- Real-time bidirectional serial communication
- Automatic port detection from target configuration
- Debug info integration for panic address resolution
- Cross-platform support (Linux, macOS)
- Graceful exit with Ctrl-C

## Examples

```bash
# Native development
llgo build hello.go                              # -> hello
llgo build -o myapp hello.go                     # -> myapp
llgo run hello.go                                # run locally
llgo install hello.go                            # install to bin

# Cross-compilation
llgo build -target pico .                        # -> hello.elf
llgo build -target pico -obin .                  # -> hello.elf + hello.bin
llgo build -target pico -ohex .                  # -> hello.elf + hello.bin + hello.hex
llgo build -target pico -ouf2 .                  # -> hello.elf + hello.uf2
llgo run -target pico .                          # run on Raspberry Pi Pico (guess a port)
llgo run -target riscv-qemu -emulator .          # run in emulator
llgo test -target pico -port /dev/ttyACM0 .      # run tests on device
llgo test -target riscv-qemu -emulator .         # run tests in emulator
llgo install -target pico -port /dev/ttyACM0 .   # flash to specific port

# Monitor device output
llgo monitor -port /dev/ttyACM0                  # monitor with specific port
llgo monitor -target pico                        # monitor with auto-detected port
llgo monitor -target pico -baudrate 9600         # custom baudrate
llgo monitor -port /dev/ttyACM0 ./firmware.elf   # with debug info for panic resolution
```
