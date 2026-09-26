// Command webview-rust 是「路线 C：Rust 壳(wry) + 系统 WebView2」的判别实验壳。
//
// 它做的事（全程自动化，无人值守、无人工读数）：
//   1. 从进程启动计时，加载 http://127.0.0.1:PORT/（IDE 形态 React 压测页，两侧同一份产物）
//   2. 轮询页面 window.__bench.readyAt，得到「冷启动 → 首屏可见」的墙钟时间
//   3. 注入同步基准（纯 JS 计算 / 强制重排 / 样式重算 / 滚动路径 / DOM 规模）
//   4. 注入 React 更新吞吐基准，等结果落地
//   5. 抓取完整 window.__bench JSON + 采样 msedgewebview2 进程组工作集
//   6. 写 JSON 报告到 out/rust-side.json 并退出
//
// 用法：webview-rust [url] [outPath]

use std::fs;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use tao::dpi::LogicalSize;
use tao::event::{Event, WindowEvent};
use tao::event_loop::{ControlFlow, EventLoop};
use tao::window::WindowBuilder;
use wry::WebViewBuilder;

// ---------- 注入脚本（与 wb-ui 侧完全一致，保证对比公平）----------
const JS_READY: &str = "(window.__bench && window.__bench.readyAt) || 0";
const JS_FATAL: &str = "(window.__bench && window.__bench.fatal) || ''";
const JS_START: &str = "(function(){try{\
  window.__countDom();\
  window.__runJs();\
  window.__runReflow(300);\
  window.__runStyleRecalc(300);\
  window.__runScroll(300);\
  window.__runReactUpdate(24);\
  return 'started';\
}catch(e){window.__bench.fatal=String((e&&e.stack)||e);return 'err';}})()";
const JS_REACT_DONE: &str = "(window.__bench.reactUpdate ? 1 : 0)";
const JS_FULL: &str = "(function(){var b=window.__bench;\
  b.domCount=document.getElementsByTagName('*').length;\
  b.mem=(window.performance&&performance.memory)?performance.memory.usedJSHeapSize:0;\
  b.title=document.title;\
  return JSON.stringify(b);})()";

#[derive(Default)]
struct Shared {
    last_poll: Option<Instant>,
    phase: u32, // 0=等 ready 1=注入同步基准 2=等 React 结果 3=收尾
    ready_ms: f64,
    page_ready_at: f64,
    benches_json: String,
    fatal: String,
    mem: String,
    finished: bool,
}

// unquote 把 WebView2 eval 回调返回的 JSON 文本还原为裸值。
fn unquote(s: &str) -> String {
    let t = s.trim();
    let t = t.strip_prefix('"').unwrap_or(t);
    let t = t.strip_suffix('"').unwrap_or(t);
    t.replace("\\\"", "\"").replace("\\\\", "\\").replace("\\n", "\n")
}

// sample_mem 只统计**本进程树**（自己 + 递归子进程）的工作集。
// 早期版本用 `Get-Process msedgewebview2` 统计全系统同名进程，会把其它应用的
// WebView2 一并算进来（实测虚高到 3.2GB），因此改为按 ParentProcessId 递归。
fn sample_mem(root_pid: u32) -> String {
    let ps = format!(
        "$root={root}; $all=Get-CimInstance Win32_Process; \
         $ids=New-Object 'System.Collections.Generic.HashSet[int]'; [void]$ids.Add($root); \
         $c=$true; while($c){{ $c=$false; foreach($p in $all){{ \
           if($ids.Contains([int]$p.ParentProcessId)){{ if($ids.Add([int]$p.ProcessId)){{ $c=$true }} }} }} }}; \
         $s=0; $n=0; $w=0; $wc=0; \
         foreach($x in $ids){{ $pr=Get-Process -Id $x -ErrorAction SilentlyContinue; \
           if($pr){{ $s+=$pr.WorkingSet64; $n++; \
             if($pr.ProcessName -eq 'msedgewebview2'){{ $w+=$pr.WorkingSet64; $wc++ }} }} }}; \
         \"$s;$n;$w;$wc\"",
        root = root_pid
    );
    let out = std::process::Command::new("powershell")
        .args(["-NoProfile", "-NonInteractive", "-Command", &ps])
        .output();
    match out {
        Ok(o) => {
            let t = String::from_utf8_lossy(&o.stdout);
            let t = t.trim();
            let f: Vec<&str> = t.split(';').collect();
            let get = |i: usize| -> f64 {
                f.get(i).and_then(|s| s.trim().parse::<f64>().ok()).unwrap_or(0.0)
            };
            format!(
                "{{\"tree_bytes\":{:.0},\"tree_procs\":{:.0},\"webview2_bytes\":{:.0},\"webview2_procs\":{:.0}}}",
                get(0),
                get(1),
                get(2),
                get(3)
            )
        }
        Err(e) => format!("{{\"error\":\"{}\"}}", e),
    }
}

fn write_report(path: &str, shared: &Shared, t0: Instant, url: &str) {
    let total = t0.elapsed().as_secs_f64() * 1000.0;
    let doc = format!(
        "{{\"side\":\"rust-wry-webview2\",\"url\":\"{}\",\
          \"coldStartToFirstPaintMs\":{:.1},\
          \"pageReadyAtMs\":{:.1},\
          \"totalProbeMs\":{:.1},\
          \"fatal\":\"{}\",\
          \"mem\":{},\
          \"bench\":{}}}",
        url,
        shared.ready_ms,
        shared.page_ready_at,
        total,
        shared.fatal.replace('"', "'"),
        if shared.mem.is_empty() { "null".into() } else { shared.mem.clone() },
        if shared.benches_json.is_empty() { "null".into() } else { shared.benches_json.clone() },
    );
    if let Some(dir) = PathBuf::from(path).parent() {
        let _ = fs::create_dir_all(dir);
    }
    let _ = fs::write(path, &doc);
    println!("\n===== RUST SIDE REPORT ({}) =====", path);
    println!("{}", doc);
}

fn main() -> wry::Result<()> {
    let mut args = std::env::args().skip(1);
    let url = args.next().unwrap_or_else(|| "http://127.0.0.1:8099/".to_string());
    let out = args.next().unwrap_or_else(|| "out/rust-side.json".to_string());
    // 第三个参数：写报告后停留多久再退出（毫秒）。留给外部截图/目检用。
    let hold_ms: u64 = args.next().and_then(|s| s.parse().ok()).unwrap_or(600);

    let t0 = Instant::now();

    let event_loop = EventLoop::new();
    let window = WindowBuilder::new()
        .with_title("route-C-spike wry webview2")
        .with_inner_size(LogicalSize::new(1280.0, 860.0))
        .build(&event_loop)
        .expect("create window");

    let shared = Arc::new(Mutex::new(Shared::default()));
    let webview = WebViewBuilder::new()
        .with_url(url.clone())
        .with_initialization_script("window.__t0_init = Date.now();")
        .build(&window)?;

    println!("[rust] webview created at {:.1}ms, loading {}", t0.elapsed().as_secs_f64() * 1000.0, url);

    let sh = shared.clone();
    let out_path = out.clone();
    let url2 = url.clone();

    event_loop.run(move |event, _target, control_flow| {
        *control_flow = ControlFlow::WaitUntil(Instant::now() + Duration::from_millis(20));

        if let Event::WindowEvent { event: WindowEvent::CloseRequested, .. } = event {
            *control_flow = ControlFlow::ExitWithCode(0);
            return;
        }

        // 节流轮询（40ms）；每轮只发一条脚本，避免 WebView2 消息队列堆积
        let (phase, should_poll) = {
            let mut s = sh.lock().unwrap();
            if s.finished {
                (99, false)
            } else {
                let ok = match s.last_poll {
                    Some(t) => t.elapsed() >= Duration::from_millis(40),
                    None => true,
                };
                if ok {
                    s.last_poll = Some(Instant::now());
                }
                (s.phase, ok)
            }
        };

        if phase == 99 {
            *control_flow = ControlFlow::ExitWithCode(0);
            return;
        }
        if !should_poll {
            return;
        }

        match phase {
            // ---- 阶段 0：等页面 ready ----
            0 => {
                let s2 = sh.clone();
                let _ = webview.evaluate_script_with_callback(JS_READY, move |v| {
                    let raw = unquote(&v);
                    let ms: f64 = raw.parse().unwrap_or(0.0);
                    if ms > 0.0 {
                        let mut s = s2.lock().unwrap();
                        if s.phase == 0 {
                            s.page_ready_at = ms;
                            s.phase = 1;
                        }
                    } else {
                        // 顺带侦察页面致命错误（React 起不来时这是关键证据）
                        let _ = &s2;
                    }
                });
                // 页面致命错误侦察（独立回调，结果写入 fatal）
                let s3 = sh.clone();
                let _ = webview.evaluate_script_with_callback(JS_FATAL, move |v| {
                    let raw = unquote(&v);
                    if !raw.is_empty() {
                        let mut s = s3.lock().unwrap();
                        if s.fatal.is_empty() {
                            s.fatal = raw;
                        }
                    }
                });
            }
            // ---- 阶段 1：ready 已确认，记录墙钟，注入同步基准 ----
            1 => {
                let mut s = sh.lock().unwrap();
                s.ready_ms = t0.elapsed().as_secs_f64() * 1000.0;
                s.phase = 2;
                println!("[rust] page ready: wall={:.1}ms pageReadyAt={:.1}ms", s.ready_ms, s.page_ready_at);
                drop(s);
                let s2 = sh.clone();
                let _ = webview.evaluate_script_with_callback(JS_START, move |v| {
                    let raw = unquote(&v);
                    if raw == "err" {
                        let mut s = s2.lock().unwrap();
                        s.fatal = format!("{} (JS_START returned err)", s.fatal);
                    }
                });
            }
            // ---- 阶段 2：等 React 更新基准落地 ----
            2 => {
                let s2 = sh.clone();
                let _ = webview.evaluate_script_with_callback(JS_REACT_DONE, move |v| {
                    if unquote(&v) == "1" {
                        let mut s = s2.lock().unwrap();
                        if s.phase == 2 {
                            s.phase = 3;
                        }
                    }
                });
            }
            // ---- 阶段 3：抓完整结果 + 采样内存，写报告 ----
            3 => {
                let s2 = sh.clone();
                let out_c = out_path.clone();
                let url_c = url2.clone();
                let _ = webview.evaluate_script_with_callback(JS_FULL, move |v| {
                    let raw = unquote(&v);
                    {
                        let mut s = s2.lock().unwrap();
                        s.benches_json = raw; // JSON.stringify 结果的裸文本
                    }
                    // 给渲染与内存稳定留一点时间，然后采样并收尾
                    std::thread::sleep(Duration::from_millis(hold_ms));
                    let m = sample_mem(std::process::id());
                    let mut s = s2.lock().unwrap();
                    s.mem = m;
                    write_report(&out_c, &s, t0, &url_c);
                    s.finished = true;
                });
            }
            _ => {}
        }
    });
}
