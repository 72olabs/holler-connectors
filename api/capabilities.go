package api

// RoomAccessCapability: the daemon offers public rooms, join and leave, join
// requests, owner management, and the visibility field on channel.create and
// channel.configure. Clients send none of these to a daemon without it.
const RoomAccessCapability = "channel-room-access-v1"
const ManagedConversationsCapability = "managed-conversations-v1"
const ChannelRenameCapability = "channel-rename-v1"
const CapabilityBridgeCapability = "capability-bridge-v1"

// ChannelMemberPostCapability: the daemon offers channel.member_post, so the
// narrow holler_channel_post tool can post without a separate approval.
const ChannelMemberPostCapability = "channel-member-post-v1"
