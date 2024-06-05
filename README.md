# ssh-sync

This project is a file transfer service via scp written in Go.

## Getting Started

This section explains how to install and run the project on your local computer.

### Prerequisites

Go (version 1.16 or higher)

### Installing

To install the project, run the following command:

```bash
git clone https://github.com/doldam0/ssh-sync.git
cd ssh-sync
go install
```

### Running

To run the project, execute the following command:

```bash
ssh-sync
```

## Usage

This section explains how to use the project.

### Commands

- `-n` (int): Check duration. This program checks the source directory every n seconds.
- `-ignore-existing` (bool): Ignore existing files. This program does not transfer existing files if this flag is set.
- `-count` (int): Check count. This program transfers files after checking n times. If the file size is updated, the check count is reset. This option is useful for transferring large files that are updated frequently.
- `-h` (bool): Show help.
- `-v` (bool): Verbose mode. This program outputs debug messages if this flag is set.
- `<src>` (string): Source directory.
- `<dst>` (string): Remote destination directory.

### Examples

```bash
ssh-sync -n 1 --ignore-existing --count 5 -v /path/to/src user@host:/path/to/dst
```

### SSH Configuration

To use this program, you need to configure the SSH connection between the local and remote servers.

1. Generate an SSH key pair.

```bash
ssh-keygen -t rsa -b 4096 -C "<comment>"
```

1. Copy the public key to the remote server.

```bash
ssh-copy-id user@host
```

1. Test the connection.

```bash
ssh user@host
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
