package cmd

import (
	"net/http"
	_ "net/http/pprof"

	log "github.com/cihub/seelog"
)

func ConfigLogging() {
	logger, err := log.LoggerFromConfigAsString(`
<seelog>
	<outputs formatid="common">
		<console />
	</outputs>
    <formats>
        <format id="common" format="%Date(2006-01-02 15:04:05.000) [%LEVEL]%t%Msg%n"/>
    </formats>
</seelog>`)
	if err != nil {
		log.Errorf("log.LoggerFromConfigAsString() failed: %v", err)
		return
	}

	err = log.ReplaceLogger(logger)
	if err != nil {
		log.Errorf("log.ReplaceLogger() failed: %v", err)
	}
}

func StartDebugServer(addr string) {
	go func() {
		err := http.ListenAndServe(addr, nil)
		if err != nil {
			log.Errorf("failed to start debug server on %v: %v", addr, err)
		} else {
			log.Debugf("debug server started on %v", addr)
		}
	}()
}
