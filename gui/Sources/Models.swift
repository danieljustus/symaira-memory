import Foundation

public struct Memory: Codable, Identifiable, Hashable {
    public let id: UUID
    public let content: String
    public let scope: String
    public let metadata: [String: String]?
    public let createdAt: Date
    public let updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case content
        case scope
        case metadata
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

public struct Rule: Codable, Identifiable, Hashable {
    public let id: UUID
    public let content: String
    public let scope: String
    public let metadata: [String: String]?
    public let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case content
        case scope
        case metadata
        case createdAt = "created_at"
    }
}

public struct StatusResponse: Codable {
    public let status: String
    public let version: String
    public let server: String
}

public struct SearchRequest: Codable {
    public let query: String
    public let scope: String?
    public let limit: Int?
}

public struct SetRequest: Codable {
    public let content: String
    public let scope: String
    public let metadata: [String: String]?
}

public struct DeleteResponse: Codable {
    public let deleted: Bool
}

public struct RulesResponse: Codable {
    public let rules: [Rule]
}

public struct ErrorResponse: Codable {
    public let error: String
}
