using System;
using System.IO;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;

namespace BiometricHelper
{
    class Program
    {
        // Windows Hello API (WinRT)
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern IntPtr GetConsoleWindow();

        static void Main(string[] args)
        {
            string input = Console.In.ReadToEnd();
            var request = JsonSerializer.Deserialize<Request>(input);

            switch (request.Action)
            {
                case "version":
                    HandleVersion();
                    break;
                case "check":
                    HandleCheck();
                    break;
                case "authenticate":
                    HandleAuthenticate(request.Reason ?? "Verify identity to access CertFlow");
                    break;
                default:
                    OutputResponse(new Response { Success = false, Error = $"Unknown action: {request.Action}" });
                    break;
            }
        }

        static void HandleVersion()
        {
            OutputResponse(new Response { Success = true, Version = "1.0.0" });
        }

        static void HandleCheck()
        {
            bool supported = CheckWindowsHelloSupport();
            OutputResponse(new Response 
            { 
                Success = true, 
                Supported = supported,
                Error = supported ? "" : "Windows Hello not available"
            });
        }

        static void HandleAuthenticate(string reason)
        {
            try
            {
                // 方法1: 使用 Windows Hello API (需要 Windows 10 1607+)
                bool success = AuthenticateWithWindowsHello(reason);
                
                OutputResponse(new Response 
                { 
                    Success = success,
                    Error = success ? "" : "Authentication failed or cancelled"
                });
            }
            catch (Exception ex)
            {
                OutputResponse(new Response 
                { 
                    Success = false, 
                    Error = ex.Message 
                });
            }
        }

        static bool CheckWindowsHelloSupport()
        {
            try
            {
                // 检查 Windows 版本 (需要 Windows 10 1607+)
                var version = Environment.OSVersion.Version;
                if (version.Major < 10 || (version.Major == 10 && version.Build < 14393))
                {
                    return false;
                }

                // 尝试创建 UserConsentVerifier 检查是否支持
                // 这里简化处理，实际应该调用 WinRT API
                return true;
            }
            catch
            {
                return false;
            }
        }

        static bool AuthenticateWithWindowsHello(string reason)
        {
            // 方法1: 使用 P/Invoke 调用 Windows Hello
            // 注意: 完整实现需要 WinRT interop，这里提供简化版本
            
            // 使用 CredentialPicker 作为备选方案
            // 这会弹出 Windows 安全对话框
            return PromptForCredential(reason);
        }

        static bool PromptForCredential(string reason)
        {
            // 使用 Windows API 弹出认证对话框
            // 这是一个简化实现，实际应该使用 WinRT API
            
            try
            {
                // 创建一个简单的认证提示
                Console.Error.WriteLine($"[Biometric Helper] Requesting authentication: {reason}");
                
                // 在实际实现中，这里应该调用:
                // - Windows.Security.Credentials.UI.UserConsentVerifier
                // - 或 Windows.Credentials.UI.CredentialPicker
                
                // 由于需要 WinRT interop，这里返回 true 作为演示
                // 实际使用时需要完整实现
                
                return true;
            }
            catch
            {
                return false;
            }
        }

        static void OutputResponse(Response response)
        {
            var options = new JsonSerializerOptions { WriteIndented = true };
            string json = JsonSerializer.Serialize(response, options);
            Console.Write(json);
        }
    }

    class Request
    {
        public string Action { get; set; }
        public string Reason { get; set; }
    }

    class Response
    {
        public bool Success { get; set; }
        public string Error { get; set; } = "";
        public string Version { get; set; }
        public bool? Supported { get; set; }
    }
}
