import requests
from datetime import datetime, timedelta

# --- 설정 정보 (월패드 환경에 맞게 수정하세요) ---
WALLPAD_IP = "10.0.0.2"
CES_PHP_PATH = "/ces/ces.php"
MAC_ADDRESS = "000b4243e742" # 예시 MAC 주소 - 실제 월패드 MAC 주소로 변경 필요
DEVICE_ID = "H6D"             # 예시 장치 ID - 실제 ID로 변경 필요
BUILDING_DONG = "109"         # 예시 동 - 실제 동으로 변경 필요
HOUSE_HO = "1201"             # 예시 호 - 실제 호로 변경 필요

# --- RAW XML 응답 가져오기 함수 ---

def get_wallpad_raw_xml(function_call: str) -> str or None:
    """
    월패드에 SOAP 요청을 보내고 RAW XML 응답 문자열을 반환합니다.
    데이터 파싱은 수행하지 않습니다.
    """
    url = f"http://{WALLPAD_IP}{CES_PHP_PATH}"
    
    soap_body = f"""
    <v:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance" xmlns:d="http://www.w3.org/2001/XMLSchema" xmlns:c="http://schemas.xmlsoap.org/soap/encoding/" xmlns:v="http://schemas.xmlsoap.org/soap/envelope/">
      <v:Header />
      <v:Body>
        <n0:callEDS id="o0" c:root="1" xmlns:n0="urn:ces">
          <in i:type="d:string">call {function_call}</in>
        </n0:callEDS>
      </v:Body>
    </v:Envelope>
    """

    headers = {
        "User-Agent": "kSOAP/2.0",
        "SOAPAction": "",
        "Content-Type": "text/xml",
        "Connection": "close",
        "Content-Length": str(len(soap_body.strip().encode('utf-8'))),
        "Host": WALLPAD_IP,
        "Accept": "*, */*"
    }

    print(f"\n--- API 호출 시도: {function_call} ---")
    print(f"  URL: {url}")
    print(f"  Request Body Preview: {soap_body.strip()[:200]}...") # 요청 본문 앞부분만 출력

    try:
        response = requests.post(url, headers=headers, data=soap_body.strip(), timeout=10)
        response.raise_for_status() # HTTP 오류 발생 시 예외 발생 (4xx, 5xx)
        print(f"  HTTP Status Code: {response.status_code} {response.reason}")
        return response.text
    except requests.exceptions.RequestException as e:
        print(f"  !!! HTTP 요청 오류 발생: {e}")
        return None
    except Exception as e:
        print(f"  !!! 알 수 없는 오류 발생: {e}")
        return None

# --- 메인 실행 로직 ---
if __name__ == "__main__":
    current_date = datetime.now()
    
    # 테스트를 위한 기준 날짜 (2025년 7월로 고정, 필요시 today = datetime.now() 사용)
    # 현재는 2025년 7월 11일이므로 이 날짜를 기준으로 합니다.
    target_year = "2025"
    target_month = "07"
    target_day = "11" # 오늘 날짜 (2025년 7월 11일)
    
    target_year_month = f"{target_year}{target_month}"
    target_year_month_day = f"{target_year}{target_month}{target_day}"
    
    energy_types = ["Electricity", "Gas", "Water"]

    print(f"--- 월패드 에너지 RAW XML 응답 전체 조회 스크립트 시작 ---")
    print(f"  대상: {BUILDING_DONG}동 {HOUSE_HO}호 (MAC: {MAC_ADDRESS}, ID: {DEVICE_ID})")
    print(f"  기준 조회 기간: {target_year}년 {target_month}월 {target_day}일\n")

    for e_type in energy_types:
        print(f"\n##################### {e_type} RAW DATA #####################")

        # 1. proc_energy_usage (월별 총 사용량)
        print(f"\n--- proc_energy_usage ({e_type}, {target_year_month}) ---")
        func_call_monthly_total = f"proc_energy_usage('{MAC_ADDRESS}', '{DEVICE_ID}', '{BUILDING_DONG}', '{HOUSE_HO}', '{e_type}', '{target_year_month}')"
        raw_xml = get_wallpad_raw_xml(func_call_monthly_total)
        if raw_xml:
            print("\n  RAW XML Response:")
            print(raw_xml)
        else:
            print(f"\n  {e_type} proc_energy_usage 응답을 가져오지 못했습니다.")

        # 2. proc_get_daily_energy (일별 사용량)
        print(f"\n--- proc_get_daily_energy ({e_type}, {target_year_month}) ---")
        func_call_daily = f"proc_get_daily_energy('{MAC_ADDRESS}', '{DEVICE_ID}', '{BUILDING_DONG}', '{HOUSE_HO}', '{e_type}', '{target_year_month}')"
        raw_xml = get_wallpad_raw_xml(func_call_daily)
        if raw_xml:
            print("\n  RAW XML Response:")
            print(raw_xml)
        else:
            print(f"\n  {e_type} proc_get_daily_energy 응답을 가져오지 못했습니다.")

        # 3. proc_get_complex_average_energy (일별 복합 평균 사용량)
        print(f"\n--- proc_get_complex_average_energy ({e_type}, {target_year_month}) ---")
        func_call_daily_avg = f"proc_get_complex_average_energy('{MAC_ADDRESS}', '{DEVICE_ID}', '{BUILDING_DONG}', '{HOUSE_HO}', '{e_type}', '{target_year_month}')"
        raw_xml = get_wallpad_raw_xml(func_call_daily_avg)
        if raw_xml:
            print("\n  RAW XML Response:")
            print(raw_xml)
        else:
            print(f"\n  {e_type} proc_get_complex_average_energy 응답을 가져오지 못했습니다.")

        # 4. proc_get_hour_energy (시간별 사용량 - 특정 일)
        print(f"\n--- proc_get_hour_energy ({e_type}, {target_year_month_day}) ---")
        func_call_hourly = f"proc_get_hour_energy('{MAC_ADDRESS}', '{DEVICE_ID}', '{BUILDING_DONG}', '{HOUSE_HO}', '{e_type}', '{target_year_month_day}')"
        raw_xml = get_wallpad_raw_xml(func_call_hourly)
        if raw_xml:
            print("\n  RAW XML Response:")
            print(raw_xml)
        else:
            print(f"\n  {e_type} proc_get_hour_energy 응답을 가져오지 못했습니다.")

        # 5. proc_get_energy_annual_select (연간 총 사용량)
        print(f"\n--- proc_get_energy_annual_select ({e_type}, {target_year}) ---")
        func_call_annual_total = f"proc_get_energy_annual_select('{MAC_ADDRESS}', '{DEVICE_ID}', '{BUILDING_DONG}', '{HOUSE_HO}', '{e_type}', '{target_year}')"
        raw_xml = get_wallpad_raw_xml(func_call_annual_total)
        if raw_xml:
            print("\n  RAW XML Response:")
            print(raw_xml)
        else:
            print(f"\n  {e_type} proc_get_energy_annual_select 응답을 가져오지 못했습니다.")

        # 6. proc_get_energy_complex_annual_select (연간 복합 평균 사용량)
        print(f"\n--- proc_get_energy_complex_annual_select ({e_type}, {target_year}) ---")
        func_call_annual_avg = f"proc_get_energy_complex_annual_select('{MAC_ADDRESS}', '{DEVICE_ID}', '{BUILDING_DONG}', '{HOUSE_HO}', '{e_type}', '{target_year}')"
        raw_xml = get_wallpad_raw_xml(func_call_annual_avg)
        if raw_xml:
            print("\n  RAW XML Response:")
            print(raw_xml)
        else:
            print(f"\n  {e_type} proc_get_energy_complex_annual_select 응답을 가져오지 못했습니다.")
            
        print("\n" + "="*80 + "\n") # 각 에너지 타입 구분선

    print("--- 월패드 에너지 RAW XML 응답 전체 조회 스크립트 완료 ---")