#!/usr/bin/env bash
set -euo pipefail

extension_id="${LINX_EXTENSION_ID:-ophjlpahpchlmihnnnihgmmeilfjmjjc}"
profile_dir="${LINX_CHROME_PROFILE:-}"
debug_port="${LINX_DEBUG_PORT:-9222}"
debug_endpoint="http://127.0.0.1:$debug_port"
auto_install_extension="${LINX_AUTO_INSTALL_EXTENSION:-1}"
auto_install_chromium="${LINX_AUTO_INSTALL_CHROMIUM:-1}"
provided_extension_dir="${LINX_EXTENSION_DIR:-}"
headless=0
setup=0
stop=0
refresh_extension=0
browser_args=()

if [[ ! "$debug_port" =~ ^[0-9]+$ ]] ||
  ((debug_port < 1 || debug_port > 65535)); then
  echo "LINX_DEBUG_PORT ต้องเป็นเลขพอร์ต 1-65535" >&2
  exit 2
fi
if [[ ! "$extension_id" =~ ^[a-p]{32}$ ]]; then
  echo "LINX_EXTENSION_ID ต้องเป็น Chrome extension ID ที่ถูกต้อง" >&2
  exit 2
fi
if [[ "$auto_install_extension" != "0" &&
  "$auto_install_extension" != "1" ]]; then
  echo "LINX_AUTO_INSTALL_EXTENSION ต้องเป็น 0 หรือ 1" >&2
  exit 2
fi
if [[ "$auto_install_chromium" != "0" &&
  "$auto_install_chromium" != "1" ]]; then
  echo "LINX_AUTO_INSTALL_CHROMIUM ต้องเป็น 0 หรือ 1" >&2
  exit 2
fi

start_url="chrome-extension://$extension_id/index.html#popout"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --setup)
      setup=1
      shift
      ;;
    --headless)
      headless=1
      shift
      ;;
    --no-auto-extension)
      auto_install_extension=0
      shift
      ;;
    --no-auto-chromium)
      auto_install_chromium=0
      shift
      ;;
    --refresh-extension)
      refresh_extension=1
      shift
      ;;
    --stop)
      stop=1
      shift
      ;;
    --)
      shift
      browser_args+=("$@")
      break
      ;;
    *)
      browser_args+=("$1")
      shift
      ;;
  esac
done

if [[ "$setup" == "1" ]]; then
  if [[ "$headless" == "1" ]]; then
    echo "--setup ต้องใช้หน้าต่าง Chrome เพื่อติดตั้ง LINE extension" >&2
    exit 2
  fi
  auto_install_extension=0
  start_url="https://chromewebstore.google.com/detail/line/$extension_id"
fi

browser=()
selected_flatpak_id=""
supports_unpacked_extension=0
requested_flatpak_id="${LINX_FLATPAK_ID:-}"

profile_contains_store_extension() {
  local directory="$1"
  local manifest
  for manifest in "$directory/Default/Extensions/$extension_id"/*/manifest.json; do
    [[ -f "$manifest" ]] && return 0
  done
  return 1
}

candidate_profile_dir() {
  local flatpak_id="${1:-}"
  if [[ -n "$profile_dir" ]]; then
    printf '%s\n' "$profile_dir"
  elif [[ -n "$flatpak_id" ]]; then
    printf '%s\n' "$HOME/.var/app/$flatpak_id/config/linx-chrome"
  else
    printf '%s\n' "$HOME/.local/share/linx-chrome"
  fi
}

select_native_browser() {
  local candidate="$1"
  command -v "$candidate" >/dev/null 2>&1 || return 1
  browser=("$candidate")
  selected_flatpak_id=""
  supports_unpacked_extension=0
  if [[ "$candidate" == "chromium" || "$candidate" == "chromium-browser" ]]; then
    supports_unpacked_extension=1
  fi
}

select_flatpak_browser() {
  local flatpak_id="$1"
  command -v flatpak >/dev/null 2>&1 || return 1
  flatpak info "$flatpak_id" >/dev/null 2>&1 || return 1
  browser=(flatpak run "$flatpak_id")
  selected_flatpak_id="$flatpak_id"
  supports_unpacked_extension=0
  if [[ "$flatpak_id" == "org.chromium.Chromium" ]]; then
    supports_unpacked_extension=1
  fi
}

install_user_chromium_flatpak() {
  local flathub_url="https://dl.flathub.org/repo/flathub.flatpakrepo"
  if ! command -v flatpak >/dev/null 2>&1; then
    echo "ติดตั้ง Chromium อัตโนมัติไม่ได้: เครื่องนี้ไม่มี Flatpak" >&2
    return 1
  fi

  if ! flatpak remotes --user --columns=name 2>/dev/null |
    sed 's/[[:space:]]*$//' |
    grep -Fxq flathub; then
    echo "กำลังเพิ่ม Flathub สำหรับผู้ใช้ปัจจุบัน..."
    if ! flatpak remote-add --user --if-not-exists flathub "$flathub_url"; then
      echo "เพิ่ม Flathub แบบ user-local ไม่สำเร็จ" >&2
      return 1
    fi
  fi

  echo "กำลังติดตั้ง Chromium แบบ user-local สำหรับ LINE zero-click..."
  if ! flatpak install --user --assumeyes --noninteractive \
    flathub org.chromium.Chromium; then
    echo "ติดตั้ง Chromium Flatpak ไม่สำเร็จ" >&2
    return 1
  fi
  flatpak info org.chromium.Chromium >/dev/null 2>&1
}

zero_click_mode=0
if [[ "$stop" == "0" && "$setup" == "0" &&
  "$auto_install_extension" == "1" ]]; then
  zero_click_mode=1
fi

if [[ -n "$requested_flatpak_id" ]]; then
  if ! select_flatpak_browser "$requested_flatpak_id"; then
    if [[ "$requested_flatpak_id" == "org.chromium.Chromium" &&
      "$zero_click_mode" == "1" && "$auto_install_chromium" == "1" ]] &&
      install_user_chromium_flatpak; then
      select_flatpak_browser "$requested_flatpak_id"
    else
      echo "ไม่พบ Flatpak browser $requested_flatpak_id" >&2
      exit 1
    fi
  fi
elif [[ "$zero_click_mode" == "1" ]]; then
  # Preserve an existing LINE profile before choosing a fresh Chromium profile.
  for candidate in google-chrome google-chrome-stable chromium chromium-browser; do
    if command -v "$candidate" >/dev/null 2>&1 &&
      profile_contains_store_extension "$(candidate_profile_dir)"; then
      select_native_browser "$candidate"
      break
    fi
  done
  if [[ ${#browser[@]} -eq 0 ]] && command -v flatpak >/dev/null 2>&1; then
    for flatpak_id in \
      com.google.Chrome \
      com.google.ChromeDev \
      org.chromium.Chromium; do
      if flatpak info "$flatpak_id" >/dev/null 2>&1 &&
        profile_contains_store_extension \
          "$(candidate_profile_dir "$flatpak_id")"; then
        select_flatpak_browser "$flatpak_id"
        break
      fi
    done
  fi

  # A new profile needs Chromium because branded Google Chrome 137+ ignores
  # --load-extension. Prefer an existing Chromium and bootstrap Flatpak as the
  # portable user-local fallback.
  if [[ ${#browser[@]} -eq 0 ]]; then
    for candidate in chromium chromium-browser; do
      if select_native_browser "$candidate"; then
        break
      fi
    done
  fi
  if [[ ${#browser[@]} -eq 0 ]]; then
    select_flatpak_browser org.chromium.Chromium || true
  fi
  if [[ ${#browser[@]} -eq 0 && "$auto_install_chromium" == "1" ]] &&
    install_user_chromium_flatpak; then
    select_flatpak_browser org.chromium.Chromium
  fi
  if [[ ${#browser[@]} -eq 0 && "$auto_install_chromium" == "1" ]]; then
    echo "เตรียม Chromium สำหรับโหมด zero-click ไม่สำเร็จ" >&2
    echo "ติดตั้ง Flatpak หรือ Chromium แล้วเรียก script นี้อีกครั้ง" >&2
    exit 1
  fi
fi

if [[ ${#browser[@]} -eq 0 ]]; then
  for candidate in google-chrome google-chrome-stable chromium chromium-browser; do
    if select_native_browser "$candidate"; then
      break
    fi
  done
fi

if [[ ${#browser[@]} -eq 0 ]] && command -v flatpak >/dev/null 2>&1; then
  for flatpak_id in \
    com.google.Chrome \
    com.google.ChromeDev \
    org.chromium.Chromium; do
    if select_flatpak_browser "$flatpak_id"; then
      break
    fi
  done
fi

if [[ ${#browser[@]} -eq 0 ]]; then
  echo "ไม่พบ Google Chrome หรือ Chromium ทั้งแบบ native และ Flatpak" >&2
  exit 1
fi

if [[ -z "$profile_dir" ]]; then
  if [[ -n "$selected_flatpak_id" ]]; then
    # Flatpak cannot see ~/.local/share by default. Keep the dedicated profile
    # in the app's persistent config directory instead.
    profile_dir="$HOME/.var/app/$selected_flatpak_id/config/linx-chrome"
  else
    profile_dir="$HOME/.local/share/linx-chrome"
  fi
fi

extension_id_from_manifest() {
  local manifest="$1"
  local key digest
  key="$(
    sed -nE 's/^[[:space:]]*"key"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/p' \
      "$manifest" | head -n 1
  )"
  [[ -n "$key" ]] || return 1
  digest="$(
    printf '%s' "$key" |
      base64 --decode 2>/dev/null |
      sha256sum |
      cut -c1-32
  )"
  [[ "$digest" =~ ^[0-9a-f]{32}$ ]] || return 1
  printf '%s' "$digest" | tr '0123456789abcdef' 'abcdefghijklmnop'
}

valid_unpacked_extension() {
  local directory="$1"
  local manifest="$directory/manifest.json"
  [[ -f "$manifest" ]] || return 1
  [[ "$(extension_id_from_manifest "$manifest")" == "$extension_id" ]]
}

profile_has_store_extension() {
  local manifest
  for manifest in "$profile_dir/Default/Extensions/$extension_id"/*/manifest.json; do
    [[ -f "$manifest" ]] && return 0
  done
  return 1
}

read_crx_varint() {
  local index="$1"
  local limit="$2"
  local value=0
  local shift=0
  local byte

  while ((index < limit && shift <= 63)); do
    byte="${crx_header_bytes[index]}"
    value=$((value | ((byte & 127) << shift)))
    index=$((index + 1))
    if ((byte < 128)); then
      crx_varint_value="$value"
      crx_varint_next="$index"
      return 0
    fi
    shift=$((shift + 7))
  done
  return 1
}

extract_crx3_proof_key() {
  local crx_file="$1"
  local destination="$2"
  local proof_index="$3"
  local proof_end="$4"
  local tag field wire length key_start candidate_id

  while ((proof_index < proof_end)); do
    read_crx_varint "$proof_index" "$proof_end" || return 1
    tag="$crx_varint_value"
    proof_index="$crx_varint_next"
    field=$((tag >> 3))
    wire=$((tag & 7))
    case "$wire" in
      0)
        read_crx_varint "$proof_index" "$proof_end" || return 1
        proof_index="$crx_varint_next"
        ;;
      1)
        proof_index=$((proof_index + 8))
        ;;
      2)
        read_crx_varint "$proof_index" "$proof_end" || return 1
        length="$crx_varint_value"
        key_start="$crx_varint_next"
        if ((key_start + length > proof_end)); then
          return 1
        fi
        if ((field == 1)); then
          dd if="$crx_file" of="$destination" bs=1 \
            skip="$((12 + key_start))" count="$length" status=none
          candidate_id="$(
            sha256sum "$destination" |
              cut -c1-32 |
              tr '0123456789abcdef' 'abcdefghijklmnop'
          )"
          if [[ "$candidate_id" == "$extension_id" ]]; then
            return 0
          fi
        fi
        proof_index=$((key_start + length))
        ;;
      5)
        proof_index=$((proof_index + 4))
        ;;
      *)
        return 1
        ;;
    esac
  done
  return 1
}

extract_crx_public_key() {
  local crx_file="$1"
  local destination="$2"
  local version header_size index limit tag field wire length payload_start
  local public_key_size candidate_id
  local -a crx_header_bytes

  version="$(od -An -tu4 -j4 -N4 "$crx_file" | tr -d '[:space:]')"
  if [[ "$version" == "2" ]]; then
    public_key_size="$(od -An -tu4 -j8 -N4 "$crx_file" | tr -d '[:space:]')"
    dd if="$crx_file" of="$destination" bs=1 skip=16 \
      count="$public_key_size" status=none
    candidate_id="$(
      sha256sum "$destination" |
        cut -c1-32 |
        tr '0123456789abcdef' 'abcdefghijklmnop'
    )"
    [[ "$candidate_id" == "$extension_id" ]]
    return
  fi
  [[ "$version" == "3" ]] || return 1

  header_size="$(od -An -tu4 -j8 -N4 "$crx_file" | tr -d '[:space:]')"
  read -r -a crx_header_bytes <<<"$(
    od -An -v -tu1 -j12 -N"$header_size" "$crx_file" | tr '\n' ' '
  )"
  index=0
  limit="${#crx_header_bytes[@]}"
  while ((index < limit)); do
    read_crx_varint "$index" "$limit" || return 1
    tag="$crx_varint_value"
    index="$crx_varint_next"
    field=$((tag >> 3))
    wire=$((tag & 7))
    case "$wire" in
      0)
        read_crx_varint "$index" "$limit" || return 1
        index="$crx_varint_next"
        ;;
      1)
        index=$((index + 8))
        ;;
      2)
        read_crx_varint "$index" "$limit" || return 1
        length="$crx_varint_value"
        payload_start="$crx_varint_next"
        if ((payload_start + length > limit)); then
          return 1
        fi
        # CRX3 fields 2 and 3 are RSA/ECDSA AsymmetricKeyProof messages.
        if ((field == 2 || field == 3)) &&
          extract_crx3_proof_key \
            "$crx_file" \
            "$destination" \
            "$payload_start" \
            "$((payload_start + length))"; then
          return 0
        fi
        index=$((payload_start + length))
        ;;
      5)
        index=$((index + 4))
        ;;
      *)
        return 1
        ;;
    esac
  done
  return 1
}

extract_crx() {
  local crx_file="$1"
  local zip_file="$2"
  local destination="$3"
  local magic version header_size public_key_size signature_size zip_offset

  magic="$(od -An -tx1 -N4 "$crx_file" | tr -d '[:space:]')"
  [[ "$magic" == "43723234" ]] || {
    echo "ไฟล์ที่ดาวน์โหลดมาไม่ใช่ Chrome extension (CRX)" >&2
    return 1
  }
  version="$(od -An -tu4 -j4 -N4 "$crx_file" | tr -d '[:space:]')"
  case "$version" in
    3)
      header_size="$(od -An -tu4 -j8 -N4 "$crx_file" | tr -d '[:space:]')"
      zip_offset=$((12 + header_size))
      ;;
    2)
      public_key_size="$(od -An -tu4 -j8 -N4 "$crx_file" | tr -d '[:space:]')"
      signature_size="$(od -An -tu4 -j12 -N4 "$crx_file" | tr -d '[:space:]')"
      zip_offset=$((16 + public_key_size + signature_size))
      ;;
    *)
      echo "ไม่รองรับ CRX version $version" >&2
      return 1
      ;;
  esac

  dd if="$crx_file" of="$zip_file" bs=1 skip="$zip_offset" status=none
  unzip -tq "$zip_file" >/dev/null
  mkdir -p "$destination"
  unzip -q "$zip_file" -d "$destination"
}

download_line_extension() {
  local destination="$1"
  local staging browser_version crx_url downloaded_id downloaded_version
  local extension_key manifest temporary_manifest
  local required_command

  for required_command in curl unzip od dd base64 sha256sum; do
    if ! command -v "$required_command" >/dev/null 2>&1; then
      echo "ติดตั้ง LINE extension อัตโนมัติไม่ได้: ไม่พบ $required_command" >&2
      echo "ติดตั้ง dependency นี้ หรือใช้ ./scripts/launch-chrome.sh --setup" >&2
      return 1
    fi
  done

  browser_version="$("${browser[@]}" --version 2>/dev/null |
    sed -nE 's/.* ([0-9]+(\.[0-9]+){1,3}).*/\1/p' | head -n 1)"
  if [[ -z "$browser_version" ]]; then
    echo "อ่าน version ของ Chrome ไม่สำเร็จ" >&2
    return 1
  fi

  staging="$profile_dir/linx-extension-download.$$"
  find "$staging" -depth -delete 2>/dev/null || true
  mkdir -p "$staging/unpacked"
  crx_url="https://clients2.google.com/service/update2/crx?response=redirect"
  crx_url+="&prodversion=$browser_version&acceptformat=crx2,crx3"
  crx_url+="&x=id%3D$extension_id%26uc"

  echo "กำลังดาวน์โหลด LINE extension จาก Chrome Web Store..."
  if ! curl -fL --retry 2 --connect-timeout 10 --max-time 90 \
    -o "$staging/line.crx" "$crx_url"; then
    find "$staging" -depth -delete 2>/dev/null || true
    echo "ดาวน์โหลด LINE extension ไม่สำเร็จ" >&2
    return 1
  fi
  if ! extract_crx_public_key \
    "$staging/line.crx" \
    "$staging/public-key.der"; then
    find "$staging" -depth -delete 2>/dev/null || true
    echo "ปฏิเสธ CRX ที่ public key ไม่ตรงกับ LINE extension ID" >&2
    return 1
  fi
  if ! extract_crx \
    "$staging/line.crx" \
    "$staging/line.zip" \
    "$staging/unpacked"; then
    find "$staging" -depth -delete 2>/dev/null || true
    return 1
  fi

  manifest="$staging/unpacked/manifest.json"
  downloaded_id="$(extension_id_from_manifest "$manifest" || true)"
  if [[ -z "$downloaded_id" ]]; then
    extension_key="$(base64 <"$staging/public-key.der" | tr -d '\r\n')"
    temporary_manifest="$staging/manifest.json"
    sed "1a\\  \"key\": \"$extension_key\"," \
      "$manifest" >"$temporary_manifest"
    mv "$temporary_manifest" "$manifest"
    downloaded_id="$(extension_id_from_manifest "$manifest" || true)"
  fi
  if [[ "$downloaded_id" != "$extension_id" ]]; then
    find "$staging" -depth -delete 2>/dev/null || true
    echo "ปฏิเสธ extension ที่ public key ไม่ตรงกับ LINE extension ID" >&2
    return 1
  fi
  downloaded_version="$(
    sed -nE 's/^[[:space:]]*"version"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/p' \
      "$manifest" | head -n 1
  )"

  if [[ -e "$destination" ]]; then
    find "$destination" -depth -delete
  fi
  mv "$staging/unpacked" "$destination"
  find "$staging" -depth -delete
  echo "ติดตั้ง LINE extension ${downloaded_version:-สำเร็จ} อัตโนมัติแล้ว"
}

loaded_extension_dir=""
if [[ "$stop" == "0" && "$setup" == "0" && -n "$provided_extension_dir" ]]; then
  if [[ "$supports_unpacked_extension" != "1" ]]; then
    echo "Google Chrome รุ่นปัจจุบันไม่รองรับ --load-extension" >&2
    echo "ใช้ Chromium หรือเอา LINX_EXTENSION_DIR ออกแล้วติดตั้งผ่าน Chrome Web Store" >&2
    exit 1
  fi
  if ! valid_unpacked_extension "$provided_extension_dir"; then
    echo "LINX_EXTENSION_DIR ไม่มี LINE extension ที่ ID ตรงกับ $extension_id" >&2
    exit 1
  fi
  loaded_extension_dir="$provided_extension_dir"
elif [[ "$stop" == "0" && "$setup" == "0" &&
  "$auto_install_extension" == "1" ]]; then
  auto_extension_dir="$profile_dir/linx-extension"
  if ! profile_has_store_extension; then
    if [[ "$supports_unpacked_extension" == "1" ]]; then
      if [[ "$refresh_extension" == "1" ]] ||
        ! valid_unpacked_extension "$auto_extension_dir"; then
        download_line_extension "$auto_extension_dir"
      fi
      loaded_extension_dir="$auto_extension_dir"
    elif [[ "$headless" == "1" ]]; then
      echo "Google Chrome รุ่นปัจจุบันปิดการติดตั้ง extension อัตโนมัติ" >&2
      echo "เปิด ./scripts/launch-chrome.sh หนึ่งครั้งแล้วกด Add to Chrome ก่อนใช้ headless" >&2
      exit 1
    else
      start_url="https://chromewebstore.google.com/detail/line/$extension_id"
      echo "Google Chrome ต้องให้ผู้ใช้ยืนยันการติดตั้ง LINE extension หนึ่งครั้ง"
      echo "กด Add to Chrome ในหน้าที่กำลังจะเปิด แล้วเรียก script นี้อีกครั้ง"
    fi
  fi
fi

runtime_dir="$profile_dir/linx-runtime"
pid_file="$runtime_dir/headless.pid"
log_file="$runtime_dir/headless.log"
unit_name="linx-headless-chrome-$debug_port.service"

find_browser_pid() {
  local cmdline pid executable raw_cmdline
  for cmdline in /proc/[0-9]*/cmdline; do
    raw_cmdline="$(tr '\0' ' ' 2>/dev/null <"$cmdline")" || continue
    [[ -n "$raw_cmdline" ]] || continue
    executable="${raw_cmdline%% *}"
    case "${executable##*/}" in
      chrome | google-chrome | chromium) ;;
      *) continue ;;
    esac
    # Flatpak Chromium exposes its complete command line as one /proc argv
    # entry, while native Chrome uses NUL-separated entries. The normalized
    # string handles both forms.
    if [[ " $raw_cmdline" == *" --user-data-dir=$profile_dir "* &&
      " $raw_cmdline" == *" --remote-debugging-port=$debug_port "* &&
      " $raw_cmdline" != *" --type="* ]]; then
      pid="${cmdline#/proc/}"
      printf '%s\n' "${pid%/cmdline}"
      return 0
    fi
  done
  return 1
}

if [[ "$stop" == "1" ]]; then
  found=0
  if command -v systemctl >/dev/null 2>&1 &&
    systemctl --user is-active --quiet "$unit_name"; then
    systemctl --user stop "$unit_name" || true
    found=1
  fi
  pid=""
  if [[ -f "$pid_file" ]]; then
    pid="$(<"$pid_file")"
  fi
  if [[ ! "$pid" =~ ^[0-9]+$ ]] || ! kill -0 "$pid" >/dev/null 2>&1; then
    pid="$(find_browser_pid || true)"
  fi
  if [[ -n "$pid" ]]; then
    found=1
    kill "$pid" >/dev/null 2>&1 || true
    for _ in {1..50}; do
      if ! curl --max-time 1 -fsS "$debug_endpoint/json/version" >/dev/null 2>&1; then
        break
      fi
      sleep 0.1
    done
    if curl --max-time 1 -fsS "$debug_endpoint/json/version" >/dev/null 2>&1; then
      kill -KILL "$pid" >/dev/null 2>&1 || true
    fi
  fi
  rm -f "$pid_file"
  if [[ "$found" == "0" ]]; then
    echo "ไม่พบ headless Chrome ที่ LINX เป็นผู้เปิด"
    exit 0
  fi
  echo "ปิด LINX headless Chrome แล้ว"
  exit 0
fi

if command -v curl >/dev/null 2>&1 &&
  curl --max-time 1 -fsS "$debug_endpoint/json/version" >/dev/null 2>&1; then
  if [[ "$headless" == "1" ]]; then
    echo "ใช้ Chrome debugger ที่ทำงานอยู่แล้วที่ $debug_endpoint"
    exit 0
  fi
  encoded_url="${start_url//#/%23}"
  curl --max-time 5 -fsS -X PUT \
    "$debug_endpoint/json/new?$encoded_url" >/dev/null
  echo "ใช้ Chrome debugger เดิมที่ $debug_endpoint และเปิด $start_url แล้ว"
  exit 0
fi

start_args=("--app=$start_url")
if [[ "$start_url" == https://* ]]; then
  start_args=("$start_url")
fi
if [[ "$headless" == "1" ]]; then
  # Unified headless Chrome supports installed extensions. A normal tab URL is
  # more reliable than app-window mode when no display server is present.
  start_args=("$start_url")
fi

command=(
  "${browser[@]}"
  --user-data-dir="$profile_dir"
  --remote-debugging-address=127.0.0.1
  --remote-debugging-port="$debug_port"
  --disable-vulkan
)
if [[ -n "$loaded_extension_dir" ]]; then
  command+=("--load-extension=$loaded_extension_dir")
fi
if [[ "$headless" == "1" ]]; then
  command+=("--headless=new" "--no-first-run" "--no-default-browser-check")
fi
command+=("${start_args[@]}" "${browser_args[@]}")

if [[ "${LINX_PRINT_COMMAND:-0}" == "1" ]]; then
  printf '%q ' "${command[@]}"
  printf '\n'
  exit 0
fi

if [[ "$headless" == "1" ]]; then
  mkdir -p "$runtime_dir"
  rm -f "$log_file"
  managed_by_systemd=0
  if command -v systemd-run >/dev/null 2>&1 &&
    systemctl --user show-environment >/dev/null 2>&1; then
    # Type=simple keeps Flatpak's launcher and its sandbox in the same
    # service. Type=exec can finish the transient unit just after startup.
    systemd-run --user --quiet --collect \
      --unit="$unit_name" \
      --property=Type=simple \
      --property="StandardOutput=append:$log_file" \
      --property="StandardError=append:$log_file" \
      -- "${command[@]}"
    managed_by_systemd=1
    pid="$(systemctl --user show "$unit_name" --property=MainPID --value)"
  else
    nohup "${command[@]}" </dev/null >"$log_file" 2>&1 &
    pid=$!
  fi
  printf '%s\n' "$pid" >"$pid_file"

  for _ in {1..80}; do
    if command -v curl >/dev/null 2>&1 &&
      curl --max-time 1 -fsS "$debug_endpoint/json/version" >/dev/null 2>&1; then
      browser_pid="$(find_browser_pid || true)"
      if [[ -n "$browser_pid" ]]; then
        pid="$browser_pid"
        printf '%s\n' "$pid" >"$pid_file"
      fi
      echo "LINX headless Chrome พร้อมที่ $debug_endpoint (PID $pid)"
      echo "log: $log_file"
      exit 0
    fi
    process_running=0
    browser_pid="$(find_browser_pid || true)"
    if [[ -n "$browser_pid" ]] && kill -0 "$browser_pid" >/dev/null 2>&1; then
      process_running=1
    elif [[ "$managed_by_systemd" == "1" ]] &&
      systemctl --user is-active --quiet "$unit_name"; then
      process_running=1
    elif [[ "$managed_by_systemd" == "0" ]] &&
      kill -0 "$pid" >/dev/null 2>&1; then
      process_running=1
    fi
    if [[ "$process_running" == "0" ]]; then
      rm -f "$pid_file"
      echo "headless Chrome หยุดทำงานก่อน CDP พร้อมใช้งาน" >&2
      tail -n 20 "$log_file" >&2 || true
      exit 1
    fi
    sleep 0.25
  done

  if [[ "$managed_by_systemd" == "1" ]]; then
    systemctl --user stop "$unit_name" >/dev/null 2>&1 || true
  else
    kill "$pid" >/dev/null 2>&1 || true
  fi
  rm -f "$pid_file"
  echo "รอ Chrome debugger ที่ $debug_endpoint ไม่สำเร็จ" >&2
  tail -n 20 "$log_file" >&2 || true
  exit 1
fi

exec "${command[@]}"
