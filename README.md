# gracehttp
http grace restart module


router := &http.ServeMux{}
cer, err := tls.LoadX509KeyPair("config/server.crt", "config/server.key")
if err != nil {
    log.Println(err)
    return
}
cfg := &tls.Config{
    Certificates:             []tls.Certificate{cer},
    MinVersion:               tls.VersionTLS12,
    CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
    PreferServerCipherSuites: true,
    CipherSuites: []uint16{
        tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
        tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
        tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
        tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
        tls.TLS_RSA_WITH_AES_256_CBC_SHA,
    },
}

srv := &http.Server{
    Addr:         ":443",
    Handler:      router,
    TLSConfig:    cfg,
    WriteTimeout: 60 * time.Second,
    ReadTimeout:  60 * time.Second,
}
http2.ConfigureServer(srv, nil)
a := gracehttp.InitHandler(srv)
a.ServerRun()
