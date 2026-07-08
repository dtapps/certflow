#!/usr/bin/env swift

import LocalAuthentication
import Foundation

// MARK: - 数据结构

struct Request: Codable {
    let action: String
    let reason: String?
}

struct Response: Codable {
    let success: Bool
    let error: String
    let version: String?
    let supported: Bool?
}

// MARK: - 主函数

func main() {
    // 从 stdin 读取请求
    let inputData = FileHandle.standardInput.readDataToEndOfFile()
    
    guard let request = try? JSONDecoder().decode(Request.self, from: inputData) else {
        let response = Response(success: false, error: "Invalid request format", version: nil, supported: nil)
        outputResponse(response)
        return
    }
    
    switch request.action {
    case "version":
        handleVersion()
    case "check":
        handleCheck()
    case "authenticate":
        handleAuthenticate(reason: request.reason ?? "Verify identity to access CertFlow")
    default:
        let response = Response(success: false, error: "Unknown action: \(request.action)", version: nil, supported: nil)
        outputResponse(response)
    }
}

// MARK: - 处理函数

func handleVersion() {
    let response = Response(success: true, error: "", version: "1.0.0", supported: nil)
    outputResponse(response)
}

func handleCheck() {
    let context = LAContext()
    var error: NSError?
    
    let supported = context.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &error)
    
    // 获取详细错误信息
    var errorDetail = ""
    if let error = error {
        errorDetail = "Code: \(error.code), Domain: \(error.domain), Description: \(error.localizedDescription)"
    }
    
    let response = Response(
        success: true,
        error: errorDetail,
        version: nil,
        supported: supported
    )
    outputResponse(response)
}

func handleAuthenticate(reason: String) {
    let context = LAContext()
    var error: NSError?
    
    // 检查是否支持
    guard context.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &error) else {
        let response = Response(
            success: false,
            error: error?.localizedDescription ?? "Biometric authentication not available",
            version: nil,
            supported: false
        )
        outputResponse(response)
        return
    }
    
    // 使用信号量等待异步回调
    let semaphore = DispatchSemaphore(value: 0)
    var authResult = false
    var authError = ""
    
    // 触发生物识别验证
    context.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: reason) { success, error in
        authResult = success
        if let error = error {
            authError = error.localizedDescription
        }
        semaphore.signal()
    }
    
    // 等待结果，最多 30 秒
    let timeout = semaphore.wait(timeout: .now() + 30)
    
    if timeout == .timedOut {
        let response = Response(
            success: false,
            error: "Authentication timed out",
            version: nil,
            supported: nil
        )
        outputResponse(response)
        return
    }
    
    let response = Response(
        success: authResult,
        error: authError,
        version: nil,
        supported: nil
    )
    outputResponse(response)
}

// MARK: - 输出函数

func outputResponse(_ response: Response) {
    let encoder = JSONEncoder()
    encoder.outputFormatting = .prettyPrinted
    
    guard let data = try? encoder.encode(response) else {
        let errorResponse: [String: Any] = ["success": false, "error": "Failed to encode response"]
        if let errorData = try? JSONSerialization.data(withJSONObject: errorResponse) {
            FileHandle.standardOutput.write(errorData)
        }
        return
    }
    
    FileHandle.standardOutput.write(data)
}

// 运行主函数
main()
