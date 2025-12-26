package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"

	"github.com/mrmahile/axion/banner"
)

// VPS represents a VPS configuration entry
type VPS struct {
	Name     string `yaml:"name"`
	IP       string `yaml:"ip"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Secret   string `yaml:"secret"` // Placeholder for future SSH key support
}

// Result represents the execution result for a VPS
type Result struct {
	VPS     VPS
	Success bool
	Stdout  string
	Stderr  string
	Error   error
}

const configPath = "/root/.config/vps/config.yaml"

// ConfigFile represents the config file structure (supports both formats)
type ConfigFile struct {
	Credentials []VPS `yaml:"credentials"`
}

// loadConfig reads and parses the YAML configuration file
func loadConfig(path string) ([]VPS, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config file not found at %s", path)
	}

	var vpsList []VPS

	// Try parsing as simple list first
	if err := yaml.Unmarshal(data, &vpsList); err != nil {
		// If that fails, try parsing with credentials wrapper
		var configFile ConfigFile
		if err2 := yaml.Unmarshal(data, &configFile); err2 != nil {
			return nil, fmt.Errorf("failed to parse config: %v (also tried credentials format: %v)", err, err2)
		}
		vpsList = configFile.Credentials
	}

	// Validate entries
	for i, vps := range vpsList {
		if vps.IP == "" {
			return nil, fmt.Errorf("VPS entry %d: IP is required", i+1)
		}
		if vps.Username == "" {
			return nil, fmt.Errorf("VPS entry %d: username is required", i+1)
		}
		if vps.Password == "" {
			return nil, fmt.Errorf("VPS entry %d: password is required", i+1)
		}
	}

	return vpsList, nil
}

// extractNumberFromName extracts the numeric part from a VPS name (e.g., "worker60" -> 60)
func extractNumberFromName(name string) (int, error) {
	// Match one or more digits at the end of the name
	re := regexp.MustCompile(`(\d+)$`)
	matches := re.FindStringSubmatch(name)
	if len(matches) < 2 {
		return 0, fmt.Errorf("no number found in VPS name: %s", name)
	}
	num, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("failed to parse number from name %s: %v", name, err)
	}
	return num, nil
}

// findVPSByNumber finds a VPS by the number in its name
func findVPSByNumber(vpsList []VPS, number int) (*VPS, error) {
	for i := range vpsList {
		num, err := extractNumberFromName(vpsList[i].Name)
		if err != nil {
			continue // Skip entries without numbers
		}
		if num == number {
			return &vpsList[i], nil
		}
	}
	return nil, fmt.Errorf("VPS with number %d not found", number)
}

// findVPSInRange finds all VPS entries whose numbers fall within the given range
func findVPSInRange(vpsList []VPS, start, end int) ([]VPS, error) {
	var matched []VPS
	for i := range vpsList {
		num, err := extractNumberFromName(vpsList[i].Name)
		if err != nil {
			continue // Skip entries without numbers
		}
		if num >= start && num <= end {
			matched = append(matched, vpsList[i])
		}
	}
	if len(matched) == 0 {
		return nil, fmt.Errorf("no VPS entries found in range %d-%d", start, end)
	}
	return matched, nil
}

// parseCommaSeparatedIndices parses a comma-separated list of indices (e.g., "52,42,53")
func parseCommaSeparatedIndices(indicesStr string) ([]int, error) {
	parts := strings.Split(indicesStr, ",")
	var indices []int
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		num, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid index '%s': %v", part, err)
		}
		if num < 1 {
			return nil, fmt.Errorf("index must be >= 1, got %d", num)
		}
		indices = append(indices, num)
	}
	if len(indices) == 0 {
		return nil, fmt.Errorf("no valid indices provided")
	}
	return indices, nil
}

// findVPSByIndices finds multiple VPS entries by their numbers
func findVPSByIndices(vpsList []VPS, indices []int) ([]VPS, error) {
	var matched []VPS
	var notFound []int

	for _, index := range indices {
		vps, err := findVPSByNumber(vpsList, index)
		if err != nil {
			notFound = append(notFound, index)
			continue
		}
		matched = append(matched, *vps)
	}

	if len(notFound) > 0 {
		return matched, fmt.Errorf("VPS numbers not found: %v", notFound)
	}

	return matched, nil
}

// parseRange parses a range string like "1-20" into start and end indices
func parseRange(rangeStr string) (start, end int, err error) {
	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range format: expected 'start-end'")
	}

	start, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start index: %v", err)
	}

	end, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end index: %v", err)
	}

	if start < 1 {
		return 0, 0, fmt.Errorf("start index must be >= 1")
	}

	if end < start {
		return 0, 0, fmt.Errorf("end index must be >= start index")
	}

	return start, end, nil
}

// executeCommand connects to a VPS via SSH and executes a command
func executeCommand(vps VPS, command string) Result {
	result := Result{
		VPS: vps,
	}

	// Build SSH client config
	config := &ssh.ClientConfig{
		User: vps.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(vps.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Accept any host key
	}

	// Connect to SSH server
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", vps.IP), config)
	if err != nil {
		result.Error = fmt.Errorf("failed to connect: %v", err)
		result.Success = false
		return result
	}
	defer client.Close()

	// Create session
	session, err := client.NewSession()
	if err != nil {
		result.Error = fmt.Errorf("failed to create session: %v", err)
		result.Success = false
		return result
	}
	defer session.Close()

	// Capture stdout and stderr
	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		result.Error = fmt.Errorf("failed to get stdout pipe: %v", err)
		result.Success = false
		return result
	}

	stderrPipe, err := session.StderrPipe()
	if err != nil {
		result.Error = fmt.Errorf("failed to get stderr pipe: %v", err)
		result.Success = false
		return result
	}

	// Execute command
	if err := session.Start(command); err != nil {
		result.Error = fmt.Errorf("failed to start command: %v", err)
		result.Success = false
		return result
	}

	// Read stdout and stderr
	var stdoutBuilder, stderrBuilder strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(&stdoutBuilder, stdoutPipe)
	}()

	go func() {
		defer wg.Done()
		io.Copy(&stderrBuilder, stderrPipe)
	}()

	// Wait for command to complete
	err = session.Wait()
	wg.Wait()

	result.Stdout = stdoutBuilder.String()
	result.Stderr = stderrBuilder.String()

	if err != nil {
		// Check if it's an ExitError (command failed but connection succeeded)
		if exitErr, ok := err.(*ssh.ExitError); ok {
			result.Error = fmt.Errorf("command exited with code %d", exitErr.ExitStatus())
			result.Success = false
		} else {
			result.Error = fmt.Errorf("command execution error: %v", err)
			result.Success = false
		}
		return result
	}

	result.Success = true
	return result
}

// printResult prints a formatted result
func printResult(result Result) {
	status := "SUCCESS"
	if !result.Success {
		status = "FAILED"
	}

	fmt.Printf("[%s] %s\n", result.VPS.Name, status)

	if result.Stdout != "" {
		fmt.Println("STDOUT:")
		fmt.Println(result.Stdout)
	}

	if result.Stderr != "" {
		fmt.Println("STDERR:")
		fmt.Println(result.Stderr)
	}

	if result.Error != nil && result.Success == false {
		if result.Stderr == "" {
			fmt.Println("STDERR:")
		}
		fmt.Printf("%v\n", result.Error)
	}
}

func main() {
	// Parse CLI flags
	var indexFlag = flag.String("i", "", "VPS index(es): single number or comma-separated (e.g., 42 or 52,42,53)")
	var rangeFlag = flag.String("l", "", "VPS range (e.g., 1-20)")
	var commandFlag = flag.String("c", "", "Command to execute (required)")
	var silent = flag.Bool("silent", false, "Silent mode.")
	var version = flag.Bool("version", false, "Print the version of the tool and exit.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEither -i or -l must be provided (not both).\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  %s -i 42 -c \"uptime\"\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -i 52,42,53 -c \"tmux ls\"\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -l 1-20 -c \"df -h\"\n", os.Args[0])
	}

	flag.Parse()

	// Print version and exit if -version flag is provided
	if *version {
		banner.PrintBanner()
		banner.PrintVersion()
		return
	}

	// Don't Print banner if -silnet flag is provided
	if !*silent {
		banner.PrintBanner()
	}

	// Validate arguments
	if *indexFlag == "" && *rangeFlag == "" {
		fmt.Fprintf(os.Stderr, "Error: either -i or -l must be provided\n")
		flag.Usage()
		os.Exit(1)
	}

	if *indexFlag != "" && *rangeFlag != "" {
		fmt.Fprintf(os.Stderr, "Error: -i and -l cannot be used together\n")
		flag.Usage()
		os.Exit(1)
	}

	if *commandFlag == "" {
		fmt.Fprintf(os.Stderr, "Error: -c is required and must be non-empty\n")
		flag.Usage()
		os.Exit(1)
	}

	// Load config
	vpsList, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(vpsList) == 0 {
		fmt.Fprintf(os.Stderr, "Error: config file contains no VPS entries\n")
		os.Exit(1)
	}

	// Execute command
	if *indexFlag != "" {
		// Check if it's comma-separated or single index
		if strings.Contains(*indexFlag, ",") {
			// Multiple VPS execution - comma-separated indices
			indices, err := parseCommaSeparatedIndices(*indexFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			matchedVPS, err := findVPSByIndices(vpsList, indices)
			if err != nil {
				// Print warning but continue with found VPS
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}

			if len(matchedVPS) == 0 {
				fmt.Fprintf(os.Stderr, "Error: no VPS entries found\n")
				os.Exit(1)
			}

			// Execute commands concurrently
			var wg sync.WaitGroup
			results := make([]Result, len(matchedVPS))

			for i := range matchedVPS {
				wg.Add(1)
				currentIndex := i

				go func(idx int, vps VPS) {
					defer wg.Done()
					results[idx] = executeCommand(vps, *commandFlag)
				}(currentIndex, matchedVPS[i])
			}

			wg.Wait()

			// Print results
			for _, result := range results {
				printResult(result)
				fmt.Println() // Blank line between results
			}

			// Check if any failed
			hasFailure := false
			for _, result := range results {
				if !result.Success {
					hasFailure = true
					break
				}
			}

			if hasFailure {
				os.Exit(1)
			}
		} else {
			// Single VPS execution - find by number in name
			index, err := strconv.Atoi(strings.TrimSpace(*indexFlag))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid index '%s': %v\n", *indexFlag, err)
				os.Exit(1)
			}

			vps, err := findVPSByNumber(vpsList, index)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			result := executeCommand(*vps, *commandFlag)
			printResult(result)

			if !result.Success {
				os.Exit(1)
			}
		}
	} else {
		// Multiple VPS execution - find by number range in names
		start, end, err := parseRange(*rangeFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		matchedVPS, err := findVPSInRange(vpsList, start, end)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Execute commands concurrently
		var wg sync.WaitGroup
		results := make([]Result, len(matchedVPS))

		for i := range matchedVPS {
			wg.Add(1)
			currentIndex := i

			go func(idx int, vps VPS) {
				defer wg.Done()
				results[idx] = executeCommand(vps, *commandFlag)
			}(currentIndex, matchedVPS[i])
		}

		wg.Wait()

		// Print results
		for _, result := range results {
			printResult(result)
			fmt.Println() // Blank line between results
		}

		// Check if any failed
		hasFailure := false
		for _, result := range results {
			if !result.Success {
				hasFailure = true
				break
			}
		}

		if hasFailure {
			os.Exit(1)
		}
	}
}
