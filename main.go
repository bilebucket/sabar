package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

const (
	// updateInterval determines the time waited between bar updates
	updateInterval time.Duration = time.Second * 3
)

func check(err error) {
	if err != nil {
		log.Printf("%v", err)
	}
}

func initializeLogging() (*os.File, error) {
	fp, err := os.OpenFile("/tmp/sabar.log", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	log.SetOutput(fp)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Llongfile)
	log.Println("Logging initialized")
	return fp, nil
}

func progressBar(value, max, width int, fore, back rune) string {
	var b strings.Builder

	filled := width * value / max

	for i := 0; i < width; i++ {
		if i <= filled {
			b.WriteRune(fore)
		} else {
			b.WriteRune(back)
		}
	}

	return b.String()
}

func sparkline(value, max int) string {
	var chars = []rune{
		'▁',
		'▂',
		'▃',
		'▄',
		'▅',
		'▆',
		'▇',
		'█',
	}

	spark := (len(chars) - 1) * value / max

	return fmt.Sprintf("%c", chars[spark])
}

type barSection func() string
type bar []barSection

func (b bar) render() string {
	var output strings.Builder
	for _, section := range b {
		output.WriteString(section())
	}
	return output.String()
}

func memUsage() string {
	memInfo, err := mem.VirtualMemory()
	check(err)
	usage := memInfo.UsedPercent

	return fmt.Sprintf("R %v %.1f%%", sparkline(int(usage), 100), usage)
}

func cpuUsage() string {
	percentages, err := cpu.Percent(0, false)
	check(err)
	usage := percentages[0]

	return fmt.Sprintf("C %v %.1f%%", sparkline(int(usage), 100), usage)
}

var oldLoad float64 = 0.0

func loadAvg() string {
	loadStat, err := load.Avg()
	check(err)

	var direction rune = '免'
	var l float64 = loadStat.Load1

	if l > oldLoad {
		direction = '勤'
	} else if l == oldLoad {
		direction = '勉'
	}

	oldLoad = l
	return fmt.Sprintf("%c%.2f", direction, l)
}

func temperatures() string {
	temperatures, err := host.SensorsTemperatures()
	// gopsutil generates warnings for sensors that do not return a
	// temperature reading. Some sensors stop measuring when the component
	// is switched off (some wireless lan chips for example).
	// We don't care about those warnings...
	if _, ok := err.(*host.Warnings); !ok {
		check(err)
	}
	var highest float64 = 0.0
	for _, t := range temperatures {
		if t.Temperature > highest {
			highest = t.Temperature
		}
	}
	return fmt.Sprintf("%.0f°C", highest)
}

func diskUsage() string {
	usage, err := disk.Usage("/")
	check(err)
	return fmt.Sprintf("[/ %0.1f%%]", usage.UsedPercent)
}

func battery() string {
	var out bytes.Buffer

	cmd := exec.Command("acpi", "-b")
	cmd.Stdout = &out

	err := cmd.Run()
	check(err)

	output := strings.Split(out.String(), ",")[1]
	output = strings.Trim(output, " ")
	output = strings.Replace(output, "%", "", -1)
	output = strings.Replace(output, "\n", "", -1)

	batteryPercentage, err := strconv.Atoi(output)
	check(err)

	return fmt.Sprintf("%s %d%%", sparkline(batteryPercentage, 100), batteryPercentage)
}

func ssid() string {
	var out bytes.Buffer

	cmd := exec.Command("iwgetid", "-r")
	cmd.Stdout = &out

	err := cmd.Run()
	check(err)

	ssid := strings.Trim(out.String(), "\n")

	return fmt.Sprintf("%s", ssid)
}

func date() string {
	// Go's way of formatting dates is WEIRD
	return time.Now().Format("02.01.2006 15:04")
}

func bspwm(c chan string) {
	var err error

	cmd := exec.Command("bspc", "subscribe", "report")
	stdout, err := cmd.StdoutPipe()
	check(err)
	scanner := bufio.NewScanner(stdout)
	err = cmd.Start()
	check(err)

	for scanner.Scan() {
		output := strings.Split(scanner.Text(), ":")
		desktopStates := output[1:]
		var b strings.Builder
		for i, ds := range desktopStates {
			state, name := ds[0], ds[1:]
			if state == 'O' || state == 'F' || state == 'U' {
				b.WriteString("%{R}")
				b.WriteString(name)
				b.WriteString("%{R}")
			} else if state == 'u' {
				b.WriteString("%{B#fe8019}%{F#000000}")
				b.WriteString(name)
				b.WriteString("%{B-}%{F-}")
			} else if state == 'o' {
				b.WriteString("%{+u}")
				b.WriteString(name)
				b.WriteString("%{-u}")
			} else if state == 'f' {
				b.WriteString(name)
			}
			if i < len(desktopStates)-1 {
				b.WriteRune(' ')
			}
		}
		c <- b.String()
	}
}

func mocp() string {
	var out bytes.Buffer

	cmd := exec.Command("mocp", "-i")
	cmd.Stdout = &out

	err := cmd.Run()
	check(err)

	output := strings.Split(out.String(), "\n")

	state := ""
	artist := ""
	title := ""
	currentSec := 0
	totalSec := 0

	for _, l := range output {
		if strings.Contains(l, "Artist: ") {
			artist = strings.Split(l, ": ")[1]
		}
		if strings.Contains(l, "SongTitle: ") {
			title = strings.Split(l, ": ")[1]
		}
		if strings.Contains(l, "State: ") {
			mocpState := strings.Split(l, ": ")[1]
			if mocpState == "PLAY" {
				state = ""
			} else if mocpState == "PAUSE" {
				state = ""
			} else {
				state = ""
			}
		}
		if strings.Contains(l, "CurrentSec: ") {
			currentSec, err = strconv.Atoi(strings.Split(l, ": ")[1])
			check(err)
		}
		if strings.Contains(l, "TotalSec: ") {
			totalSec, err = strconv.Atoi(strings.Split(l, ": ")[1])
			check(err)
		}
	}

	return fmt.Sprintf("[%s] %s %s: %s", progressBar(currentSec, totalSec, 10, '=', ' '), state, artist, title)
}

func center() string {
	return "%{c}"
}

func right() string {
	return "%{r}"
}

func spacer() string {
	return " "
}

func main() {
	logFile, err := initializeLogging()
	check(err)
	defer logFile.Close()

	var b bar = bar{
		mocp,
		center,
		date,
		right,
		loadAvg,
		spacer,
		cpuUsage,
		spacer,
		memUsage,
		spacer,
		diskUsage,
		spacer,
		temperatures,
		spacer,
		ssid,
		spacer,
		battery,
	}

	ticker := time.NewTicker(updateInterval)
	bspwmChan := make(chan string)
	go bspwm(bspwmChan)

	// Read initial desktop state
	s := <-bspwmChan

	// Render once right after staring up
	fmt.Println(b.render())

	// Loop forever and forever and render bar every updateInterval seconds
	// or when bspwm signals that we have switched desktops
	// TODO: handle os signals?
	for {
		select {
		case s = <-bspwmChan:
			fmt.Printf("%s %s\n", s, b.render())
		case <-ticker.C:
			fmt.Printf("%s %s\n", s, b.render())
		}
	}
}
