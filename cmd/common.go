package cmd

import log "github.com/cihub/seelog"

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
