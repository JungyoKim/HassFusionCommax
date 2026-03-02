import requests
import xml.etree.ElementTree as ET
from datetime import datetime, timedelta

# --- 설정 정보 (월패드 환경에 맞게 수정하세요) ---
WALLPAD_IP = "10.0.0.2"
CES_PHP_PATH = "/ces/ces.php"
MAC_ADDRESS = "000b4243e742" # 예시 MAC 주소 - 실제 월패드 MAC 주소로 변경 필요
DEVICE_ID = "H6D"             # 예시 장치 ID - 실제 ID로 변경 필요
BUILDING_DONG = "109"         # 예시 동 - 실제 동으로 변경 필요
HOUSE_HO = "1201"             # 예시 호 - 실제 호로 변경 필요

# XML 응답 파싱을 위한 네임스페이스 딕셔너리 (필요시 추가 또는 수정)
# 이전에 result 태그를 찾지 못한 문제를 해결하기 위해,
# result 태그가 ns1(urn:ces)의 자식으로 네임스페이스 없이 존재하는 것으로 가정
NAMESPACES = {'ns1': 'urn:ces'}

# --- 공통 함수 정의 ---

def send_wallpad_request(function_call: str) -> str or None:
    """
    월패드에 SOAP 요청을 보내고 XML 응답 문자열을 반환합니다.
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

    try:
        response = requests.post(url, headers=headers, data=soap_body.strip(), timeout=10) # 타임아웃 10초로 늘림
        response.raise_for_status() # HTTP 오류 발생 시 예외 발생
        return response.text
    except requests.exceptions.RequestException as e:
        print(f"  HTTP 요청 오류 발생: {e}")
        return None
    except Exception as e:
        print(f"  알 수 없는 오류 발생: {e}")
        return None

def parse_energy_result(xml_string: str) -> dict or None:
    """
    에너지 조회 응답 XML에서 result 태그의 데이터를 파싱합니다.
    """
    if not xml_string:
        return None
    try:
        root = ET.fromstring(xml_string)
        # 이전에 발생했던 네임스페이스 문제를 해결하기 위해 경로 수정
        # ns1은 urn:ces, callEDSResponse의 자식인 result 태그는 네임스페이스가 없음
        result_tag = root.find(".//ns1:callEDSResponse/result", NAMESPACES)
        
        if result_tag is not None:
            raw_data = result_tag.text
            parsed_data = {}
            if raw_data:
                entries = raw_data.split('$')
                for entry in entries:
                    if '#' in entry:
                        parts = entry.split('#')
                        if len(parts) >= 2: # 최소한 날짜와 사용량은 있어야 함
                            date_or_key = parts[0]
                            try:
                                usage = float(parts[1])
                                parsed_data[date_or_key] = usage
                            except ValueError:
                                print(f"  경고: 유효하지 않은 사용량 데이터 '{parts[1]}'를 건너뜁니다.")
            return parsed_data
        else:
            print("  응답 XML에서 result 태그를 찾을 수 없습니다. 원시 응답:")
            print(xml_string)
            return None
    except ET.ParseError as e:
        print(f"  XML 파싱 오류 발생: {e}")
        print(f"  원시 응답:\n{xml_string}")
        return None
    except Exception as e:
        print(f"  데이터 파싱 중 알 수 없는 오류 발생: {e}")
        return None

# --- 각 에너지 조회 함수 래퍼 ---

def get_proc_energy_usage(energy_type: str, year_month: str) -> dict or None:
    """월별 총 에너지 사용량을 조회합니다."""
    func_call = f"proc_energy_usage('{MAC_ADDRESS}', '{DEVICE_ID}', '{BUILDING_DONG}', '{HOUSE_HO}', '{energy_type}', '{year_month}')"
    print(f"  -> 호출: {func_call}")
    xml_response = send_wallpad_request(func_call)
    return parse_energy_result(xml_response)

def get_proc_get_daily_energy(energy_type: str, year_month: str) -> dict or None:
    """일별 에너지 사용량을 조회합니다."""
    func_call = f"proc_get_daily_energy('{MAC_ADDRESS}', '{DEVICE_ID}', '{BUILDING_DONG}', '{HOUSE_HO}', '{energy_type}', '{year_month}')"
    print(f"  -> 호출: {func_call}")
    xml_response = send_wallpad_request(func_call)
    return parse_energy_result(xml_response)

def get_proc_get_complex_average_energy(energy_type: str, year_month: str) -> dict or None:
    """일별 복합 평균 에너지 사용량을 조회합니다."""
    func_call = f"proc_get_complex_average_energy('{MAC_ADDRESS}', '{DEVICE_ID}', '{BUILDING_DONG}', '{HOUSE_HO}', '{energy_type}', '{year_month}')"
    print(f"  -> 호출: {func_call}")
    xml_response = send_wallpad_request(func_call)
    return parse_energy_result(xml_response)

def get_proc_get_hour_energy(energy_type: str, year_month_day: str) -> dict or None:
    """시간별 에너지 사용량을 조회합니다."""
    func_call = f"proc_get_hour_energy('{MAC_ADDRESS}', '{DEVICE_ID}', '{BUILDING_DONG}', '{HOUSE_HO}', '{energy_type}', '{year_month_day}')"
    print(f"  -> 호출: {func_call}")
    xml_response = send_wallpad_request(func_call)
    return parse_energy_result(xml_response)

def get_proc_get_energy_annual_select(energy_type: str, year: str) -> dict or None:
    """연간 총 에너지 사용량을 조회합니다."""
    func_call = f"proc_get_energy_annual_select('{MAC_ADDRESS}', '{DEVICE_ID}', '{BUILDING_DONG}', '{HOUSE_HO}', '{energy_type}', '{year}')"
    print(f"  -> 호출: {func_call}")
    xml_response = send_wallpad_request(func_call)
    return parse_energy_result(xml_response)

def get_proc_get_energy_complex_annual_select(energy_type: str, year: str) -> dict or None:
    """연간 복합 평균 에너지 사용량을 조회합니다."""
    func_call = f"proc_get_energy_complex_annual_select('{MAC_ADDRESS}', '{DEVICE_ID}', '{BUILDING_DONG}', '{HOUSE_HO}', '{energy_type}', '{year}')"
    print(f"  -> 호출: {func_call}")
    xml_response = send_wallpad_request(func_call)
    return parse_energy_result(xml_response)

# --- 메인 실행 로직 ---
if __name__ == "__main__":
    today = datetime.now()
    # 테스트를 위해 2025년 7월을 기준으로 설정 (사용자 요청에 따른)
    target_year = "2025"
    target_month = "07"
    target_year_month = f"{target_year}{target_month}"
    # 2025년 7월 10일 (예시로 고정, 실제는 오늘 날짜 사용 가능)
    target_day = "10" 
    target_year_month_day = f"{target_year}{target_month}{target_day}" 
    
    # 조회할 에너지 종류 목록
    energy_types = ["Electricity", "Gas", "Water"]

    print(f"--- 월패드 에너지 데이터 전체 조회 스크립트 시작 ---")
    print(f"  대상: {BUILDING_DONG}동 {HOUSE_HO}호 (MAC: {MAC_ADDRESS}, ID: {DEVICE_ID})")
    print(f"  대상 조회 기간 (예시): {target_year}년 {target_month}월 {target_day}일\n")

    for e_type in energy_types:
        print(f"### {e_type} 데이터 조회 ###")

        # 1. 월별 총 사용량
        print(f"\n[1. {e_type} 월별 총 사용량 ({target_year_month})]")
        data = get_proc_energy_usage(e_type, target_year_month)
        if data:
            for key, usage in data.items():
                print(f"  {target_year}년 {key}월 총 사용량: {usage:.3f}")
        else:
            print(f"  {e_type} 월별 총 사용량 데이터를 가져오지 못했습니다.")

        # 2. 일별 사용량
        print(f"\n[2. {e_type} 일별 사용량 ({target_year_month})]")
        data = get_proc_get_daily_energy(e_type, target_year_month)
        if data:
            # 날짜 순으로 정렬하여 출력
            sorted_data = sorted(data.items())
            for date_str, usage in sorted_data:
                print(f"  {date_str[:4]}년 {date_str[4:6]}월 {date_str[6:]}일 사용량: {usage:.3f}")
        else:
            print(f"  {e_type} 일별 사용량 데이터를 가져오지 못했습니다.")

        # 3. 일별 복합 평균 사용량
        print(f"\n[3. {e_type} 일별 복합 평균 사용량 ({target_year_month})]")
        data = get_proc_get_complex_average_energy(e_type, target_year_month)
        if data:
            sorted_data = sorted(data.items())
            for date_str, usage in sorted_data:
                print(f"  {date_str[:4]}년 {date_str[4:6]}월 {date_str[6:]}일 복합 평균: {usage:.3f}")
        else:
            print(f"  {e_type} 일별 복합 평균 사용량 데이터를 가져오지 못했습니다.")

        # 4. 시간별 사용량 (특정 일)
        print(f"\n[4. {e_type} 시간별 사용량 ({target_year_month_day})]")
        data = get_proc_get_hour_energy(e_type, target_year_month_day)
        if data:
            # 시간 순으로 정렬하여 출력 (키가 시간대일 경우)
            sorted_data = sorted(data.items(), key=lambda x: int(x[0]) if x[0].isdigit() else x[0])
            for hour_key, usage in sorted_data:
                # 시간 키가 '01', '02' 등일 경우
                if hour_key.isdigit() and len(hour_key) <= 2:
                     print(f"  {target_year_month_day[:4]}년 {target_year_month_day[4:6]}월 {target_year_month_day[6:]}일 {hour_key}시 사용량: {usage:.3f}")
                else: # 키가 시간 이외의 다른 형식일 경우 (예: 'TOTAL')
                    print(f"  {hour_key}: {usage:.3f}")
        else:
            print(f"  {e_type} 시간별 사용량 데이터를 가져오지 못했습니다.")

        # 5. 연간 총 사용량
        print(f"\n[5. {e_type} 연간 총 사용량 ({target_year})]")
        data = get_proc_get_energy_annual_select(e_type, target_year)
        if data:
            # key가 월(MM)일 경우
            if len(data) == 12: 
                sorted_data = sorted(data.items(), key=lambda x: int(x[0]))
                for month_key, usage in sorted_data:
                    print(f"  {target_year}년 {month_key}월 총 사용량: {usage:.3f}")
            else: # 그 외의 경우 (예: 연간 총합이 한 번에 나오는 경우)
                 for key, usage in data.items():
                    print(f"  {key}년 총 사용량: {usage:.3f}")
        else:
            print(f"  {e_type} 연간 총 사용량 데이터를 가져오지 못했습니다.")

        # 6. 연간 복합 평균 사용량
        print(f"\n[6. {e_type} 연간 복합 평균 사용량 ({target_year})]")
        data = get_proc_get_energy_complex_annual_select(e_type, target_year)
        if data:
            if len(data) == 12:
                sorted_data = sorted(data.items(), key=lambda x: int(x[0]))
                for month_key, usage in sorted_data:
                    print(f"  {target_year}년 {month_key}월 복합 평균: {usage:.3f}")
            else:
                for key, usage in data.items():
                    print(f"  {key}년 복합 평균: {usage:.3f}")
        else:
            print(f"  {e_type} 연간 복합 평균 사용량 데이터를 가져오지 못했습니다.")
        
        print("\n" + "="*50 + "\n") # 각 에너지 타입 구분선

    print("--- 월패드 에너지 데이터 전체 조회 스크립트 완료 ---")