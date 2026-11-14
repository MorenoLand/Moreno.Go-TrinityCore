-- Dancing Flames (Midsummer brazier) --
-- Lua port of src/server/scripts/World/npcs_special.cpp:312-412
-- (npc_dancing_flamesAI). Entry 25305 TDB-verified (ScriptName
-- npc_dancing_flames).
-- Eluna creature events: 5 OnSpawn, 7 OnUpdate, 8 OnReceiveEmote,
-- 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- Reset (:328) — DoCastSelf(45423 BRAZIER, triggered) +
-- DoCastSelf(45427 FIERY_AURA) + Relocate(x, y, z+0.94) +
-- SetDisableGravity(true) + HandleEmoteCommand(EMOTE_ONESHOT_DANCE
-- 94) + Initialize (Active=true, CanIteract=3500).
-- UpdateAI (:343) — !Active + 3500ms -> Active=true + re-dance
-- emote. Rides ev7 (100ms diff).
-- ReceiveEmote (:360) — IsWithinLOS + IsWithinDistInMap(player, 30)
-- -> SetFacingToObject(player) + Active=false; KISS 58 -> SHY 24 /
-- WAVE 101 -> WAVE 3 / BOW 17 -> BOW 2 / JOKE 329 -> LAUGH 11 /
-- DANCE 34 -> DoCast(player, 47057 SEDUCTION, triggered) if
-- !player:HasAura(47057). The empty JustEngagedWith (:358)
-- registers nothing.
-- Deviation only: BuildHeartBeatMsg/SendMessageToSet position
-- broadcast packet has no bridge (cosmetic; the relocate ports).

local ENTRY_DANCING_FLAMES = 25305

local SPELL_BRAZIER = 45423
local SPELL_FIERY_AURA = 45427
local SPELL_SEDUCTION = 47057
local EMOTE_ONESHOT_DANCE = 94
local EMOTE_ONESHOT_SHY = 24
local EMOTE_ONESHOT_WAVE = 3
local EMOTE_ONESHOT_BOW = 2
local EMOTE_ONESHOT_LAUGH = 11
local TEXT_EMOTE_KISS = 58
local TEXT_EMOTE_WAVE = 101
local TEXT_EMOTE_BOW = 17
local TEXT_EMOTE_JOKE = 329
local TEXT_EMOTE_DANCE = 34

local flamesState = {}

local function onSpawnOrReset(_, creature)
    local guid = creature:GetGUIDLow()
    flamesState[guid] = {active = true, canIteract = 3500}
    creature:CastSpell(creature, SPELL_BRAZIER, true)
    creature:CastSpell(creature, SPELL_FIERY_AURA)
    creature:Relocate(creature:GetX(), creature:GetY(), creature:GetZ() + 0.94)
    creature:SetDisableGravity(true)
    creature:PerformEmote(EMOTE_ONESHOT_DANCE)
end

-- C++ UpdateAI :343 (ev7 ticks with the 100ms diff).
local function onUpdate(_, creature, diff)
    local guid = creature:GetGUIDLow()
    local st = flamesState[guid]
    if not st or st.active then
        return
    end
    if st.canIteract <= diff then
        st.active = true
        st.canIteract = 3500
        creature:PerformEmote(EMOTE_ONESHOT_DANCE)
    else
        st.canIteract = st.canIteract - diff
    end
end

local function onReceiveEmote(_, creature, player, emote)
    local guid = creature:GetGUIDLow()
    local st = flamesState[guid]
    if not st then
        return
    end
    if creature:IsWithinLOS(player:GetX(), player:GetY(), player:GetZ())
        and creature:IsWithinDistInMap(player, 30.0) then
        creature:SetFacingToObject(player)
        st.active = false
        if emote == TEXT_EMOTE_KISS then
            creature:PerformEmote(EMOTE_ONESHOT_SHY)
        elseif emote == TEXT_EMOTE_WAVE then
            creature:PerformEmote(EMOTE_ONESHOT_WAVE)
        elseif emote == TEXT_EMOTE_BOW then
            creature:PerformEmote(EMOTE_ONESHOT_BOW)
        elseif emote == TEXT_EMOTE_JOKE then
            creature:PerformEmote(EMOTE_ONESHOT_LAUGH)
        elseif emote == TEXT_EMOTE_DANCE then
            if not player:HasAura(SPELL_SEDUCTION) then
                creature:CastSpell(player, SPELL_SEDUCTION, true)
            end
        end
    end
end

RegisterCreatureEvent(ENTRY_DANCING_FLAMES, 5, onSpawnOrReset)
RegisterCreatureEvent(ENTRY_DANCING_FLAMES, 23, onSpawnOrReset)
RegisterCreatureEvent(ENTRY_DANCING_FLAMES, 7, onUpdate)
RegisterCreatureEvent(ENTRY_DANCING_FLAMES, 8, onReceiveEmote)
