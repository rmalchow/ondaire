#include "provision.h"
#include "config.h"
#include "netif.h"

#include <string.h>
#include <stdio.h>
#include <stdlib.h>
#include <errno.h>
#include "cJSON.h"
#include "esp_log.h"
#include "esp_system.h"
#include "esp_wifi.h"
#include "esp_netif.h"
#include "esp_event.h"
#include "esp_timer.h"
#include "esp_http_server.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "lwip/sockets.h"
#include "sdkconfig.h"

static const char *TAG = "provision";

// The default AP netif hands out this address and answers as the gateway (DHCP).
#define AP_IP "192.168.4.1"

static httpd_handle_t s_http;
static esp_timer_handle_t s_life;
static volatile int s_dns_sock = -1;
static volatile bool s_dns_run;

// ---------------------------------------------------------------------------
// Captive DNS: answer every A query with the AP address so any hostname the
// client's OS probes resolves to us and its "sign-in" sheet opens. Single-question
// queries only (what phones send); anything else is echoed back unanswered.
// ---------------------------------------------------------------------------
static void dns_task(void *arg) {
    (void)arg;
    int sock = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
    if (sock < 0) { ESP_LOGE(TAG, "dns socket: %d", errno); vTaskDelete(NULL); return; }
    struct sockaddr_in me = { .sin_family = AF_INET, .sin_port = htons(53),
                              .sin_addr.s_addr = htonl(INADDR_ANY) };
    if (bind(sock, (struct sockaddr *)&me, sizeof me) < 0) {
        ESP_LOGE(TAG, "dns bind: %d", errno); close(sock); vTaskDelete(NULL); return;
    }
    s_dns_sock = sock;
    uint8_t buf[512];
    while (s_dns_run) {
        struct sockaddr_in from; socklen_t fl = sizeof from;
        int n = recvfrom(sock, buf, sizeof buf, 0, (struct sockaddr *)&from, &fl);
        if (n < 12) continue;                 // shorter than a DNS header
        if (buf[2] & 0x80) continue;          // already a response
        if (((buf[4] << 8) | buf[5]) != 1) continue;   // want exactly one question

        // Walk the (uncompressed) question name to find where to splice the answer,
        // so it lands before any EDNS OPT the client appended in the additional section.
        int p = 12;
        bool ok = true;
        while (p < n && buf[p] != 0) {
            if (buf[p] & 0xC0) { ok = false; break; }   // compression not used in queries
            p += buf[p] + 1;
        }
        if (!ok || p >= n) continue;
        int qend = p + 1 + 4;                 // terminating 0 + qtype(2) + qclass(2)
        if (qend > n || qend + 16 > (int)sizeof buf) continue;

        buf[2] = 0x81; buf[3] = 0x80;         // QR=1, RD, RA
        buf[6] = 0x00; buf[7] = 0x01;         // ANCOUNT = 1
        buf[8] = 0x00; buf[9] = 0x00;         // NSCOUNT = 0
        buf[10] = 0x00; buf[11] = 0x00;       // ARCOUNT = 0 (drop any EDNS OPT)
        uint8_t *a = buf + qend;              // answer immediately after the question
        *a++ = 0xC0; *a++ = 0x0C;             // name = pointer to the question name
        *a++ = 0x00; *a++ = 0x01;             // type A
        *a++ = 0x00; *a++ = 0x01;             // class IN
        *a++ = 0x00; *a++ = 0x00; *a++ = 0x00; *a++ = 0x3C;   // TTL 60s
        *a++ = 0x00; *a++ = 0x04;             // rdlength 4
        *a++ = 192;  *a++ = 168; *a++ = 4; *a++ = 1;          // 192.168.4.1
        sendto(sock, buf, qend + 16, 0, (struct sockaddr *)&from, fl);
    }
    close(sock);
    s_dns_sock = -1;
    vTaskDelete(NULL);
}

// ---------------------------------------------------------------------------
// HTTP portal
// ---------------------------------------------------------------------------

// In-place percent-decode (and '+' → space) of an x-www-form-urlencoded value.
static void urldecode(char *s) {
    char *o = s;
    for (char *p = s; *p; p++) {
        if (*p == '+') { *o++ = ' '; }
        else if (*p == '%' && p[1] && p[2]) {
            char h[3] = { p[1], p[2], 0 };
            *o++ = (char)strtol(h, NULL, 16);
            p += 2;
        } else { *o++ = *p; }
    }
    *o = '\0';
}

static const char PAGE_HEAD[] =
    "<!doctype html><meta name=viewport content=\"width=device-width,initial-scale=1\">"
    "<title>ensemble setup</title><style>"
    "body{font-family:system-ui,sans-serif;max-width:22em;margin:2em auto;padding:0 1em}"
    "label{display:block;margin-top:.6em}input{width:100%;padding:.5em;margin:.3em 0;"
    "box-sizing:border-box}button{width:100%;padding:.7em;font-size:1em;margin-top:1em}"
    "</style><h2>ensemble speaker</h2><form method=POST action=/save>"
    "<label>Wi-Fi network<input list=nets name=ssid autocomplete=off required></label>"
    "<datalist id=nets></datalist>"
    "<label>Password<input name=pass type=password></label>"
    "<label>Speaker name<input name=name value=\"";
static const char PAGE_TAIL[] =
    "\"></label><button>Save &amp; reboot</button></form><script>"
    "fetch('/scan').then(r=>r.json()).then(a=>{let d=document.getElementById('nets');"
    "a.forEach(n=>{let o=document.createElement('option');o.value=n.ssid;d.appendChild(o)})})"
    "</script>";

static esp_err_t h_root(httpd_req_t *req) {
    ens_config_t *g = config_get();
    httpd_resp_set_type(req, "text/html");
    httpd_resp_send_chunk(req, PAGE_HEAD, HTTPD_RESP_USE_STRLEN);
    httpd_resp_send_chunk(req, g->name, HTTPD_RESP_USE_STRLEN);   // prefill current name
    httpd_resp_send_chunk(req, PAGE_TAIL, HTTPD_RESP_USE_STRLEN);
    httpd_resp_send_chunk(req, NULL, 0);
    return ESP_OK;
}

static esp_err_t h_scan(httpd_req_t *req) {
    wifi_scan_config_t sc = { .show_hidden = false };
    cJSON *arr = cJSON_CreateArray();
    if (esp_wifi_scan_start(&sc, true) == ESP_OK) {
        uint16_t n = 0;
        esp_wifi_scan_get_ap_num(&n);
        if (n > 20) n = 20;
        wifi_ap_record_t *recs = calloc(n, sizeof *recs);
        if (recs && esp_wifi_scan_get_ap_records(&n, recs) == ESP_OK) {
            for (uint16_t i = 0; i < n; i++) {
                if (recs[i].ssid[0] == '\0') continue;
                cJSON *o = cJSON_CreateObject();
                cJSON_AddStringToObject(o, "ssid", (char *)recs[i].ssid);
                cJSON_AddNumberToObject(o, "rssi", recs[i].rssi);
                cJSON_AddNumberToObject(o, "auth", recs[i].authmode);
                cJSON_AddItemToArray(arr, o);
            }
        }
        free(recs);
    }
    char *s = cJSON_PrintUnformatted(arr);
    httpd_resp_set_type(req, "application/json");
    httpd_resp_sendstr(req, s ? s : "[]");
    cJSON_free(s);
    cJSON_Delete(arr);
    return ESP_OK;
}

static esp_err_t h_save(httpd_req_t *req) {
    char body[256];
    int len = req->content_len < (int)sizeof body - 1 ? req->content_len : (int)sizeof body - 1;
    int got = 0;
    while (got < len) {
        int r = httpd_req_recv(req, body + got, len - got);
        if (r <= 0) { httpd_resp_send_500(req); return ESP_FAIL; }
        got += r;
    }
    body[got] = '\0';

    ens_config_t *g = config_get();
    char ssid[33] = "", pass[64] = "", name[33] = "";
    if (httpd_query_key_value(body, "ssid", ssid, sizeof ssid) == ESP_OK) {
        urldecode(ssid);
        strncpy(g->wifi_ssid, ssid, sizeof g->wifi_ssid - 1);
        g->wifi_ssid[sizeof g->wifi_ssid - 1] = '\0';
    }
    if (httpd_query_key_value(body, "pass", pass, sizeof pass) == ESP_OK) {
        urldecode(pass);
        strncpy(g->wifi_pass, pass, sizeof g->wifi_pass - 1);
        g->wifi_pass[sizeof g->wifi_pass - 1] = '\0';
    }
    if (httpd_query_key_value(body, "name", name, sizeof name) == ESP_OK && name[0]) {
        urldecode(name);
        strncpy(g->name, name, sizeof g->name - 1);
        g->name[sizeof g->name - 1] = '\0';
    }

    const char *reason = NULL;
    if (g->wifi_ssid[0] == '\0') {
        httpd_resp_set_type(req, "text/html");
        httpd_resp_sendstr(req, "<meta name=viewport content=\"width=device-width\">"
                                "<p>Wi-Fi network is required. <a href=/>back</a>");
        return ESP_OK;
    }
    if (!config_validate(g, &reason) || !config_save()) {
        httpd_resp_send_500(req);
        return ESP_FAIL;
    }
    ESP_LOGI(TAG, "saved creds for \"%s\" — rebooting", g->wifi_ssid);
    httpd_resp_set_type(req, "text/html");
    httpd_resp_sendstr(req, "<meta name=viewport content=\"width=device-width\">"
                            "<h3>Saved. Rebooting\xe2\x80\xa6</h3>");
    vTaskDelay(pdMS_TO_TICKS(600));
    esp_restart();
    return ESP_OK;
}

// Everything else → bounce to the portal root so the OS captive-portal check trips.
static esp_err_t h_redirect(httpd_req_t *req, httpd_err_code_t err) {
    (void)err;
    httpd_resp_set_status(req, "302 Found");
    httpd_resp_set_hdr(req, "Location", "http://" AP_IP "/");
    httpd_resp_send(req, NULL, 0);
    return ESP_OK;
}

static void http_start(void) {
    httpd_config_t cfg = HTTPD_DEFAULT_CONFIG();
    cfg.lru_purge_enable = true;
    if (httpd_start(&s_http, &cfg) != ESP_OK) { ESP_LOGE(TAG, "httpd start failed"); return; }
    httpd_uri_t root = { .uri = "/",     .method = HTTP_GET,  .handler = h_root };
    httpd_uri_t scan = { .uri = "/scan", .method = HTTP_GET,  .handler = h_scan };
    httpd_uri_t save = { .uri = "/save", .method = HTTP_POST, .handler = h_save };
    httpd_register_uri_handler(s_http, &root);
    httpd_register_uri_handler(s_http, &scan);
    httpd_register_uri_handler(s_http, &save);
    httpd_register_err_handler(s_http, HTTPD_404_NOT_FOUND, h_redirect);
}

// ---------------------------------------------------------------------------
// Lifetime
// ---------------------------------------------------------------------------
static void provision_stop(void *arg) {
    (void)arg;
    ESP_LOGW(TAG, "provisioning window closed — inert until power cycle "
                  "(re-provision over USB, or power-cycle to retry Wi-Fi)");
    if (s_http) { httpd_stop(s_http); s_http = NULL; }
    s_dns_run = false;
    if (s_dns_sock >= 0) { shutdown(s_dns_sock, SHUT_RDWR); close(s_dns_sock); }
    esp_wifi_stop();
}

void provision_start(bool wifi_started) {
    if (wifi_started) {
        // STA is already up and failed to connect: stop it fighting us, keep the
        // interface for scanning, and add an AP alongside it.
        netif_wifi_suppress_reconnect();
        esp_wifi_disconnect();
    } else {
        // Fully unprovisioned: bring Wi-Fi up from scratch (STA iface is needed to
        // scan; AP iface serves the portal).
        ESP_ERROR_CHECK(esp_netif_init());
        esp_err_t e = esp_event_loop_create_default();
        if (e != ESP_OK && e != ESP_ERR_INVALID_STATE) ESP_ERROR_CHECK(e);
        esp_netif_create_default_wifi_sta();
        wifi_init_config_t wc = WIFI_INIT_CONFIG_DEFAULT();
        ESP_ERROR_CHECK(esp_wifi_init(&wc));
    }
    esp_netif_create_default_wifi_ap();

    char idhex[33]; config_node_id_hex(idhex);
    wifi_config_t ap = { 0 };
    int slen = snprintf((char *)ap.ap.ssid, sizeof ap.ap.ssid, "ensemble-%.4s", idhex);
    ap.ap.ssid_len = slen;
    ap.ap.channel = 1;
    ap.ap.max_connection = 4;
    ap.ap.authmode = WIFI_AUTH_OPEN;

    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_APSTA));
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_AP, &ap));
    if (!wifi_started) ESP_ERROR_CHECK(esp_wifi_start());
    esp_wifi_set_ps(WIFI_PS_NONE);
    ESP_LOGW(TAG, "captive portal up: join open AP \"%.*s\", browse to http://" AP_IP "/",
             slen, ap.ap.ssid);

    s_dns_run = true;
    xTaskCreate(dns_task, "dns", 3072, NULL, 5, NULL);
    http_start();

    const esp_timer_create_args_t ta = { .callback = provision_stop, .name = "prov-life" };
    ESP_ERROR_CHECK(esp_timer_create(&ta, &s_life));
    ESP_ERROR_CHECK(esp_timer_start_once(s_life,
        (uint64_t)CONFIG_ENSEMBLE_PORTAL_TIMEOUT_MS * 1000));
}
