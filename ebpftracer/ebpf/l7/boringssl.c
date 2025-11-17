// BoringSSL support for statically linked binaries (e.g., Envoy)
// BoringSSL is mostly API-compatible with OpenSSL, but may have structural differences
// Note: struct unused and unused_fn are defined in openssl.c

// BoringSSL BIO structure (similar to OpenSSL v1.1.1+)
// In practice, BoringSSL uses same structures as OpenSSL 1.1.1
// so we can reuse bio_st_v1_1_1 from openssl.c

// BoringSSL uses same structure layout as OpenSSL 1.1.1+
#define GET_FD_BORING(ctx, bio_rw)                                       \
({                                                                       \
    struct ssl_st ssl;                                                   \
    if (bpf_probe_read(&ssl, sizeof(ssl), (void*)PT_REGS_PARM1(ctx))) { \
        return 0;                                                        \
    };                                                                   \
    struct bio_st_v1_1_1 bio;                                            \
    if (bpf_probe_read(&bio, sizeof(bio), (void*)ssl.bio_rw)) {         \
        return 0;                                                        \
    };                                                                   \
    __u32 fd = bio.num;                                                  \
    if (fd <= 2) {                                                       \
        return 0;                                                        \
    }                                                                    \
    fd;                                                                  \
})

#define BORING_WRITE_ENTER(ctx)                                  \
({                                                               \
    __u32 fd = GET_FD_BORING(ctx, wbio);                         \
    char* buf_ptr = (char*)PT_REGS_PARM2(ctx);                   \
    __u64 buf_size = PT_REGS_PARM3(ctx);                         \
    return trace_enter_write(ctx, fd, 1, buf_ptr, buf_size, 0);  \
})

#define BORING_READ_ENTER(ctx)                            \
({                                                        \
    __u32 fd = GET_FD_BORING(ctx, rbio);                  \
    char* buf_ptr = (char*)PT_REGS_PARM2(ctx);            \
    __u64 pid_tgid = bpf_get_current_pid_tgid();          \
    __u32 pid = pid_tgid >> 32;                           \
    __u64 id = pid_tgid | IS_TLS_READ_ID;                 \
    return trace_enter_read(id, pid, fd, buf_ptr, 0, 0);  \
})

#define BORING_READ_EX_ENTER(ctx)                                   \
({                                                                  \
    __u32 fd = GET_FD_BORING(ctx, rbio);                            \
    char* buf_ptr = (char*)PT_REGS_PARM2(ctx);                      \
    __u64 pid_tgid = bpf_get_current_pid_tgid();                    \
    __u64 id = pid_tgid | IS_TLS_READ_ID;                           \
    __u32 pid = pid_tgid >> 32;                                     \
    __u64* ret_ptr = (__u64*)PT_REGS_PARM4(ctx);                    \
    return trace_enter_read(id, pid, fd, buf_ptr, ret_ptr, 0);      \
})

SEC("uprobe/boringssl_SSL_write_enter")
int boringssl_SSL_write_enter(struct pt_regs *ctx) {
    BORING_WRITE_ENTER(ctx);
}

SEC("uprobe/boringssl_SSL_read_enter")
int boringssl_SSL_read_enter(struct pt_regs *ctx) {
    BORING_READ_ENTER(ctx);
}

SEC("uprobe/boringssl_SSL_read_ex_enter")
int boringssl_SSL_read_ex_enter(struct pt_regs *ctx) {
    BORING_READ_EX_ENTER(ctx);
}

SEC("uprobe/boringssl_SSL_read_exit")
int boringssl_SSL_read_exit(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 pid = pid_tgid >> 32;
    __u64 id = pid_tgid | IS_TLS_READ_ID;
    int ret = (int)PT_REGS_RC(ctx);
    return trace_exit_read(ctx, id, pid, 1, ret);
}

