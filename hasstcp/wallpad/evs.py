# import requests
# from lxml import etree
# import sys
# import time

# # --- 설정 (Configuration) ---
# ELEVATOR_SERVER_URL = "http://10.0.0.2:29715"
# SOAP_ACTION_HEADER = "urn:ces#setEvStatus" # 엘리베이터 호출을 위한 SOAPAction, 이대로 시도해봅니다.

# # OpenWrt WAN 인터페이스에 추가된 IP 주소와 층 매핑
# # 이 부분을 실제 엘리베이터 시스템의 IP-층 매핑에 맞춰 수정해야 합니다.
# # 예시: {층수: '해당 층에 할당된 OpenWrt WAN의 별칭 IP'}
# # 현재 1층만 확실하니, 1층 IP와 기타 예시로 채워둡니다.
# # 층수 '1'은 10.9.1.147로 가정합니다. 다른 층은 패턴에 맞춰 추측합니다.
# # **이 맵을 정확히 채워주셔야 합니다.**
# FLOOR_IP_MAP = {
#     1: '10.9.1.147',   # 1층 (확실)
#     2: '10.9.2.147',   # 2층 (추측: 10.9.X.147 패턴)
#     3: '10.9.3.147',
#     # ... 필요한 층수를 여기에 추가하고, 해당하는 IP 주소를 정확히 넣어주세요.
#     # 예시: 12층이 10.9.12.11이었다면, 13층은 10.9.13.11일 수도 있습니다.
#     # 이 패턴에 대한 정확한 정보가 중요합니다!
#     13: '10.9.13.147', # 유저가 제시한 13층 예시 IP
# }

# # -----------------------------

# def send_elevator_call(target_floor, call_direction):
#     """
#     엘리베이터 호출 요청을 보냅니다.
#     요청의 출발지 IP는 목표 층에 따라 다릅니다.

#     Args:
#         target_floor (int): 엘리베이터를 호출할 목표 층 번호.
#         call_direction (str): 'up' (상승 호출) 또는 'down' (하강 호출).
#     """
#     if target_floor not in FLOOR_IP_MAP:
#         print(f"오류: {target_floor}층에 해당하는 IP 주소 정보가 없습니다. FLOOR_IP_MAP을 확인해주세요.")
#         return

#     if call_direction not in ['up', 'down']:
#         print("오류: call_direction은 'up' 또는 'down'이어야 합니다.")
#         return

#     source_ip_for_floor = FLOOR_IP_MAP[target_floor]
    
#     call_up_value = 1 if call_direction == 'up' else 0
#     call_down_value = 1 if call_direction == 'down' else 0

#     # 층 호출을 위한 SOAP 바디 (getEvStatus 때와 동일하게 isWallpad 포함)
#     soap_body = f"""<?xml version="1.0" encoding="UTF-8"?>
# <SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:SOAP-ENC="http://schemas.xmlsoap.org/soap/encoding/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:ns="urn:ces">
# <SOAP-ENV:Body SOAP-ENV:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
# <ns:setEvStatus>
#     <in xsi:type="ns:inEvStatus">
#         <callUp>{call_up_value}</callUp>
#         <callDown>{call_down_value}</callDown>
#     </in>
#     <isWallpad>1</isWallpad>
# </ns:setEvStatus>
# </SOAP-ENV:Body>
# </SOAP-ENV:Envelope>"""

#     headers = {
#         "Content-Type": "text/xml; charset=utf-8",
#         "SOAPAction": SOAP_ACTION_HEADER
#     }

#     print(f"\n엘리베이터 {target_floor}층으로 {call_direction} 호출 시도 중...")
#     print(f"출발지 IP: {source_ip_for_floor}")

#     try:
#         # source_address 파라미터를 사용하여 출발지 IP 지정
#         response = requests.post(ELEVATOR_SERVER_URL,
#                                  headers=headers,
#                                  data=soap_body,
#                                  source_address=(source_ip_for_floor, 0)) # 0은 OS가 포트 선택하도록
#         response.raise_for_status() # HTTP 오류가 발생하면 예외 발생

#         print(f"호출 요청 성공. HTTP 상태 코드: {response.status_code}")
#         print("응답 XML:")
#         print(response.text)

#         try:
#             root = etree.fromstring(response.text.encode('utf-8'))
#             namespaces = {'SOAP-ENV': "http://schemas.xmlsoap.org/soap/envelope/"}
            
#             fault = root.find('.//SOAP-ENV:Fault', namespaces)
#             if fault is not None:
#                 print("\n**SOAP 오류 응답 감지!**")
#                 for child in fault:
#                     print(f"   {child.tag.split('}')[-1]}: {child.text}")
#             else:
#                 print(f"\n엘리베이터 {target_floor}층 호출 명령이 서버에 성공적으로 전달되었습니다.")
#                 print("실제로 엘리베이터가 움직이는지 확인해주세요.")

#         except etree.XMLSyntaxError as e:
#             print(f"응답 XML 파싱 실패: {e}")
#             print("응답 내용이 유효한 XML이 아닐 수 있습니다.")

#     except requests.exceptions.RequestException as e:
#         print(f"엘리베이터 호출 요청 실패: {e}")
#     except Exception as e:
#         print(f"알 수 없는 오류 발생: {e}")

# if __name__ == "__main__":
#     print("--------------------------------------------------")
#     print("        엘리베이터 층수 조정 스크립트 by Gemini AI         ")
#     print("--------------------------------------------------")
#     print("1. 이 스크립트는 OpenWrt WAN에 등록된 별칭 IP를 사용하여 특정 층으로 엘리베이터를 호출합니다.")
#     print("2. 호출할 층과 방향을 입력하면 해당 층의 IP를 출발지 IP로 사용합니다.")
#     print("3. 'up'은 목표 층보다 높은 층에서 엘리베이터를 부르고, 'down'은 낮은 층에서 부릅니다.")
#     print("--------------------------------------------------")

#     while True:
#         try:
#             target_floor_str = input("\n호출할 목표 층수를 입력하세요 ('q' 입력 시 종료): ").strip()
#             if target_floor_str.lower() == 'q':
#                 print("스크립트를 종료합니다.")
#                 break
            
#             target_floor = int(target_floor_str)
#             if target_floor <= 0:
#                 print("유효한 층수를 입력해주세요.")
#                 continue

#             direction = input(f"{target_floor}층에서 어떤 방향으로 호출하시겠습니까? ['up' 또는 'down' 입력]: ").strip().lower()

#             send_elevator_call(target_floor, direction)
#             time.sleep(1) # 연속 호출 방지

#         except ValueError:
#             print("잘못된 입력입니다. 층수는 숫자로 입력해주세요.")
#         except Exception as e:
#             print(f"예상치 못한 오류 발생: {e}")

import requests
from lxml import etree
import sys
import time
from urllib3.util import connection
import socket

# --- 설정 (Configuration) ---
ELEVATOR_SERVER_URL = "http://10.0.0.2:29715"
SOAP_ACTION_HEADER = "urn:ces#setEvStatus"

# OpenWrt WAN 인터페이스에 추가된 IP 주소와 층 매핑
FLOOR_IP_MAP = {
    1: '10.9.1.147',   # 1층
    2: '10.9.2.147',   # 2층 (패턴 추측)
    3: '10.9.3.147',
    13: '10.9.13.147', # 13층
    # 추가 층과 IP를 여기에 입력
}

# 상수 정의
CALL_DIRECTIONS = ['up', 'down']
SUCCESS_MESSAGE = "엘리베이터 {floor}층 호출 명령이 서버에 성공적으로 전달되었습니다."
ERROR_INVALID_FLOOR = "오류: {floor}층에 해당하는 IP 주소 정보가 없습니다. FLOOR_IP_MAP을 확인해주세요."
ERROR_INVALID_DIRECTION = "오류: call_direction은 'up' 또는 'down'이어야 합니다."

# -----------------------------

def bind_source_ip(source_ip):
    """소스 IP를 바인딩하도록 urllib3의 연결 설정을 패치"""
    _original_create_connection = connection.create_connection

    def patched_create_connection(address, *args, **kwargs):
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind((source_ip, 0))  # 소스 IP와 임의 포트 바인딩
        sock.connect(address)
        return sock

    connection.create_connection = patched_create_connection
    return _original_create_connection

def send_elevator_call(target_floor, call_direction):
    """
    엘리베이터 호출 요청을 보냅니다.
    요청의 출발지 IP는 목표 층에 따라 다릅니다.

    Args:
        target_floor (int): 엘리베이터를 호출할 목표 층 번호.
        call_direction (str): 'up' (상승 호출) 또는 'down' (하강 호출).
    """
    if target_floor not in FLOOR_IP_MAP:
        print(ERROR_INVALID_FLOOR.format(floor=target_floor))
        return

    if call_direction not in CALL_DIRECTIONS:
        print(ERROR_INVALID_DIRECTION)
        return

    source_ip_for_floor = FLOOR_IP_MAP[target_floor]
    call_up_value = 1 if call_direction == 'up' else 0
    call_down_value = 1 if call_direction == 'down' else 0

    # SOAP 요청 바디
    soap_body = f"""<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:SOAP-ENC="http://schemas.xmlsoap.org/soap/encoding/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:ns="urn:ces">
<SOAP-ENV:Body SOAP-ENV:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<ns:setEvStatus>
    <in xsi:type="ns:inEvStatus">
        <callUp>{call_up_value}</callUp>
        <callDown>{call_down_value}</callDown>
    </in>
    <isWallpad>1</isWallpad>
</ns:setEvStatus>
</SOAP-ENV:Body>
</SOAP-ENV:Envelope>"""

    headers = {
        "Content-Type": "text/xml; charset=utf-8",
        "SOAPAction": SOAP_ACTION_HEADER
    }

    print(f"\n엘리베이터 {target_floor}층으로 {call_direction} 호출 시도 중...")
    print(f"출발지 IP: {source_ip_for_floor}")

    # 소스 IP 바인딩 설정
    original_create_connection = bind_source_ip(source_ip_for_floor)

    try:
        response = requests.post(ELEVATOR_SERVER_URL,
                                headers=headers,
                                data=soap_body,
                                timeout=10)  # 타임아웃 추가
        response.raise_for_status()

        print(f"호출 요청 성공. HTTP 상태 코드: {response.status_code}")
        print("응답 XML:")
        print(response.text)

        try:
            root = etree.fromstring(response.text.encode('utf-8'))
            namespaces = {'SOAP-ENV': "http://schemas.xmlsoap.org/soap/envelope/"}
            fault = root.find('.//SOAP-ENV:Fault', namespaces)
            if fault is not None:
                print("\n**SOAP 오류 응답 감지!**")
                for child in fault:
                    print(f"   {child.tag.split('}')[-1]}: {child.text}")
            else:
                print(SUCCESS_MESSAGE.format(floor=target_floor))
                print("실제로 엘리베이터가 움직이는지 확인해주세요.")

        except etree.XMLSyntaxError as e:
            print(f"응답 XML 파싱 실패: {e}")
            print("응답 내용이 유효한 XML이 아닐 수 있습니다.")

    except requests.exceptions.Timeout:
        print("요청 타임아웃: 서버에 연결할 수 없습니다. 네트워크 상태를 확인해주세요.")
    except requests.exceptions.RequestException as e:
        print(f"엘리베이터 호출 요청 실패: {e}")
    except Exception as e:
        print(f"알 수 없는 오류 발생: {e}")
    finally:
        # 원래 연결 함수로 복구
        connection.create_connection = original_create_connection

if __name__ == "__main__":
    print("--------------------------------------------------")
    print("        엘리베이터 층수 조정 스크립트         ")
    print("--------------------------------------------------")
    print("1. 이 스크립트는 OpenWrt WAN에 등록된 별칭 IP를 사용하여 특정 층으로 엘리베이터를 호출합니다.")
    print("2. 호출할 층과 방향을 입력하면 해당 층의 IP를 출발지 IP로 사용합니다.")
    print("3. 'up'은 목표 층보다 높은 층에서 엘리베이터를 부르고, 'down'은 낮은 층에서 부릅니다.")
    print("--------------------------------------------------")

    while True:
        try:
            target_floor_str = input("\n호출할 목표 층수를 입력하세요 ('q' 입력 시 종료): ").strip()
            if target_floor_str.lower() == 'q':
                print("스크립트를 종료합니다.")
                break

            target_floor = int(target_floor_str)
            if target_floor <= 0:
                print("유효한 층수를 입력해주세요.")
                continue

            direction = input(f"{target_floor}층에서 어떤 방향으로 호출하시겠습니까? ['up' 또는 'down' 입력]: ").strip().lower()
            send_elevator_call(target_floor, direction)
            time.sleep(1)  # 연속 호출 방지

        except ValueError:
            print("잘못된 입력입니다. 층수는 숫자로 입력해주세요.")
        except Exception as e:
            print(f"예상치 못한 오류 발생: {e}")