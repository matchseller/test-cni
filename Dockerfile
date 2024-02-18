FROM centos:7
COPY test-cni-ds /root/test-cni-ds
COPY test-cni /opt/cni/bin/test-cni
RUN chmod +x /root/test-cni-ds && chmod +x /opt/cni/bin/test-cni
ENTRYPOINT ["/root/test-cni-ds"]