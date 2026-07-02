import Foundation
import Combine

public enum ConnectionStatus: Equatable, Hashable {
    case disconnected
    case connecting
    case connected(version: String)
    case failed(String)
}

@MainActor
public class APIClient: ObservableObject {
    @Published public var connectionStatus: ConnectionStatus = .disconnected
    @Published public var memories: [Memory] = []
    @Published public var rules: [Rule] = []
    @Published public var isFetching = false
    @Published public var errorMessage: String? = nil
    
    @Published public var serverURL: String {
        didSet {
            UserDefaults.standard.set(serverURL, forKey: "symaira_server_url")
            Task { await checkStatus() }
        }
    }
    
    @Published public var token: String {
        didSet {
            KeychainHelper.shared.save(token)
            Task { await checkStatus() }
        }
    }
    
    public init() {
        let savedURL = UserDefaults.standard.string(forKey: "symaira_server_url") ?? "http://127.0.0.1:8787"
        self.serverURL = savedURL
        self.token = KeychainHelper.shared.read() ?? ""
    }
    
    private var jsonDecoder: JSONDecoder {
        let decoder = JSONDecoder()
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.calendar = Calendar(identifier: .gregorian)
        decoder.dateDecodingStrategy = .custom { decoder -> Date in
            let container = try decoder.singleValueContainer()
            let dateStr = try container.decode(String.self)
            
            let formats = [
                "yyyy-MM-dd'T'HH:mm:ss.SSSSSSSSSZZZZZ",
                "yyyy-MM-dd'T'HH:mm:ss.SSSSSSZZZZZ",
                "yyyy-MM-dd'T'HH:mm:ss.SSSZZZZZ",
                "yyyy-MM-dd'T'HH:mm:ssZZZZZ",
                "yyyy-MM-dd'T'HH:mm:ss.SSSSSSSSS'Z'",
                "yyyy-MM-dd'T'HH:mm:ss.SSSSSS'Z'",
                "yyyy-MM-dd'T'HH:mm:ss.SSS'Z'",
                "yyyy-MM-dd'T'HH:mm:ss'Z'",
                "yyyy-MM-dd HH:mm:ss"
            ]
            for format in formats {
                formatter.dateFormat = format
                if let date = formatter.date(from: dateStr) {
                    return date
                }
            }
            // Fallback
            return Date()
        }
        return decoder
    }
    
    private func createRequest(path: String, method: String, body: Data? = nil) -> URLRequest? {
        guard let baseURL = URL(string: serverURL) else { return nil }
        
        // Use URLComponents to safely append path to base URL without double slashes
        var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: true)
        let cleanPath = path.hasPrefix("/") ? String(path.dropFirst()) : path
        let existingPath = components?.path ?? ""
        components?.path = existingPath.appending("/\(cleanPath)")
        
        guard let url = components?.url else { return nil }
        var request = URLRequest(url: url)
        request.httpMethod = method
        
        // Add headers
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if !token.isEmpty {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        
        request.httpBody = body
        return request
    }
    
    public func checkStatus() async {
        connectionStatus = .connecting
        guard let request = createRequest(path: "api/status", method: "GET") else {
            connectionStatus = .failed("Invalid server URL")
            return
        }
        
        do {
            let (data, response) = try await URLSession.shared.data(for: request)
            guard let httpResponse = response as? HTTPURLResponse else {
                connectionStatus = .failed("Invalid server response type")
                return
            }
            
            if httpResponse.statusCode == 200 {
                let statusRes = try jsonDecoder.decode(StatusResponse.self, from: data)
                connectionStatus = .connected(version: statusRes.version)
            } else {
                connectionStatus = .failed("Server returned status code \(httpResponse.statusCode)")
            }
        } catch {
            connectionStatus = .failed(error.localizedDescription)
        }
    }
    
    public func fetchMemories(scope: String? = nil) async {
        isFetching = true
        errorMessage = nil
        
        var path = "api/list"
        if let scope = scope, !scope.isEmpty, scope.lowercased() != "all" {
            path += "?scope=\(scope.lowercased())"
        }
        
        guard let request = createRequest(path: path, method: "GET") else {
            errorMessage = "Invalid server URL"
            isFetching = false
            return
        }
        
        do {
            let (data, response) = try await URLSession.shared.data(for: request)
            guard let httpResponse = response as? HTTPURLResponse else {
                errorMessage = "Invalid response from server"
                isFetching = false
                return
            }
            
            if httpResponse.statusCode == 200 {
                let decoded = try jsonDecoder.decode([Memory].self, from: data)
                self.memories = decoded.sorted(by: { $0.createdAt > $1.createdAt })
            } else if httpResponse.statusCode == 401 {
                errorMessage = "Unauthorized. Please check your Bearer API token."
                connectionStatus = .failed("Unauthorized")
            } else {
                errorMessage = "Failed to load memories (code \(httpResponse.statusCode))"
            }
        } catch {
            errorMessage = error.localizedDescription
        }
        
        isFetching = false
    }
    
    public func searchMemories(query: String, scope: String? = nil, limit: Int = 10) async {
        guard !query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            await fetchMemories(scope: scope)
            return
        }
        
        isFetching = true
        errorMessage = nil
        
        let normalizedScope = (scope == "all" || scope == nil) ? nil : scope?.lowercased()
        let searchReq = SearchRequest(query: query, scope: normalizedScope, limit: limit)
        
        guard let bodyData = try? JSONEncoder().encode(searchReq),
              let request = createRequest(path: "api/search", method: "POST", body: bodyData) else {
            errorMessage = "Could not initialize search request"
            isFetching = false
            return
        }
        
        do {
            let (data, response) = try await URLSession.shared.data(for: request)
            guard let httpResponse = response as? HTTPURLResponse else {
                errorMessage = "Invalid response"
                isFetching = false
                return
            }
            
            if httpResponse.statusCode == 200 {
                let decoded = try jsonDecoder.decode([Memory].self, from: data)
                self.memories = decoded
            } else if httpResponse.statusCode == 401 {
                errorMessage = "Unauthorized. Please check your Bearer API token."
            } else {
                errorMessage = "Search failed (code \(httpResponse.statusCode))"
            }
        } catch {
            errorMessage = error.localizedDescription
        }
        
        isFetching = false
    }
    
    public func saveMemory(content: String, scope: String) async -> Bool {
        isFetching = true
        errorMessage = nil
        
        let setReq = SetRequest(content: content, scope: scope.lowercased(), metadata: ["source": "macOS GUI App"])
        guard let bodyData = try? JSONEncoder().encode(setReq),
              let request = createRequest(path: "api/set", method: "POST", body: bodyData) else {
            errorMessage = "Could not initialize save request"
            isFetching = false
            return false
        }
        
        do {
            let (_, response) = try await URLSession.shared.data(for: request)
            guard let httpResponse = response as? HTTPURLResponse else {
                errorMessage = "Invalid response"
                isFetching = false
                return false
            }
            
            if httpResponse.statusCode == 200 {
                isFetching = false
                await fetchMemories(scope: scope)
                return true
            } else {
                errorMessage = "Save failed (code \(httpResponse.statusCode))"
            }
        } catch {
            errorMessage = error.localizedDescription
        }
        
        isFetching = false
        return false
    }
    
    public func deleteMemory(id: UUID, currentScope: String? = nil) async -> Bool {
        isFetching = true
        errorMessage = nil
        
        guard let request = createRequest(path: "api/delete?id=\(id.uuidString)", method: "DELETE") else {
            errorMessage = "Could not initialize delete request"
            isFetching = false
            return false
        }
        
        do {
            let (_, response) = try await URLSession.shared.data(for: request)
            guard let httpResponse = response as? HTTPURLResponse else {
                errorMessage = "Invalid response"
                isFetching = false
                return false
            }
            
            if httpResponse.statusCode == 200 {
                isFetching = false
                await fetchMemories(scope: currentScope)
                return true
            } else {
                errorMessage = "Delete failed (code \(httpResponse.statusCode))"
            }
        } catch {
            errorMessage = error.localizedDescription
        }
        
        isFetching = false
        return false
    }
    
    public func fetchRules(scope: String? = nil) async {
        isFetching = true
        errorMessage = nil
        
        var path = "api/rules"
        if let scope = scope, !scope.isEmpty, scope.lowercased() != "all" {
            path += "?scope=\(scope.lowercased())"
        }
        
        guard let request = createRequest(path: path, method: "GET") else {
            errorMessage = "Invalid server URL"
            isFetching = false
            return
        }
        
        do {
            let (data, response) = try await URLSession.shared.data(for: request)
            guard let httpResponse = response as? HTTPURLResponse else {
                errorMessage = "Invalid response"
                isFetching = false
                return
            }
            
            if httpResponse.statusCode == 200 {
                let decoded = try jsonDecoder.decode(RulesResponse.self, from: data)
                self.rules = decoded.rules.sorted(by: { $0.createdAt > $1.createdAt })
            } else {
                errorMessage = "Failed to load rules (code \(httpResponse.statusCode))"
            }
        } catch {
            errorMessage = error.localizedDescription
        }
        
        isFetching = false
    }
}
