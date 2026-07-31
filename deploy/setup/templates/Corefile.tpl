${LEAK_ZONE}:53 {
    bind ${BIND_IPS}
    errors
    log
    dnstap tcp://127.0.0.1:6000 full
    file /etc/coredns/zones/${LEAK_ZONE}.db ${LEAK_ZONE}
}
