package cmds

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
	"github.com/v2rayA/v2rayA/pkg/util/log"
)

func IsCommandValid(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

func ExecCommands(commands string, stopWhenError bool) error {
	lines := strings.Split(commands, "\n")
	var e error
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) <= 0 || strings.HasPrefix(line, "#") {
			continue
		}
		out, err := exec.Command("sh", "-c", line).CombinedOutput()
		if err != nil {
			e = fmt.Errorf("ExecCommands: %v %v: %w", line, string(out), err)
			if stopWhenError {
				log.Trace("%v", e)
				return e
			}
		}
	}
	return e
}

func ExecCmd(s string) string {
	if len(s) == 0 {
		return ""
	}
	command := exec.Command("cmd.exe", "/c", s)
	log.Info("ExecCmd Run:%s", command.String())

	var buffer bytes.Buffer
	command.Stdout = &buffer //设置输入
	if err := command.Start(); err != nil {
		log.Error("ExecCmd Start err:%s", errors.WithStack(err).Error())
		return ""
	}
	if err := command.Wait(); err != nil {
		log.Error("ExecCmd Wait err:%s, buffer:%s", errors.WithStack(err).Error(), buffer.String())
		return ""
	}
	var result = buffer.String()
	buffer.Reset()

	return result
}

func ExecCmdWithArgsAsync(cmd string, args ...string) int {
	if len(cmd) == 0 {
		return 0
	}
	command := exec.Command(cmd, args...)
	log.Info("ExecCmdWithArgsAsync Run:%s", command.String())

	command.Stdout = os.Stdout //设置输入
	if err := command.Start(); err != nil {
		log.Error("ExecCmdWithArgsAsync Start err:%s", errors.WithStack(err).Error())
		return 0
	}

	return command.Process.Pid
}

func ExecCmdWithArgs(cmd string, args ...string) {
	if len(cmd) == 0 {
		return
	}
	command := exec.Command(cmd, args...)
	log.Info("ExecCmdWithArgs Run:%s", command.String())

	command.Stdout = os.Stdout //设置输入
	if err := command.Run(); err != nil {
		log.Error("ExecCmdWithArgs Run err:%s", errors.WithStack(err).Error())
		return
	}
}
