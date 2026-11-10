-- Blasted Lands — Lua port of
-- src/server/scripts/EasternKingdoms/zone_blasted_lands.cpp
-- (spell_razelikh_teleport_group, quest 3628 support; the only
-- AddSC_blasted_lands() registration).
--
-- C++ HandleScriptEffect (zone_blasted_lands.cpp:58-76): on effect 0
-- SCRIPT_EFFECT hit, GetHitPlayer() — if the player is grouped,
-- every group member within 20 yd and alive casts 13142 on
-- themselves (triggered); else the player casts 12885 on
-- themselves (triggered). Ported via RegisterSpellEvent(27686, 5):
-- SpellEventOnEffectHitTarget fires once per (effect, live hit
-- target) before the default handler (spells.go:27398, player-cast
-- funnel; boss_ai.go:3427, creature-cast path). The loader's
-- spell_script_names binding is spell 27686 (SPELL_TELEPORT_GROUP,
-- zone_blasted_lands.cpp:31) — the same enum-ID evidence standard
-- as the muru/vx_001 SpellScript ports. Group iteration via
-- Group:GetMembers() (globals.go:1791, online player objects),
-- the live twin of C++'s GetFirstMember loop (offline members have
-- no live object and cannot satisfy IsWithinDistInMap/!isDead
-- anyway). Distance twin: IsWithinDistInMap 20.0 (Object.cpp:1233)
-- = GetDistance - reach - reach < 20, player reach 1.5 both sides
-- (threatPlayersInRange convention, boss_kiljaeden.lua:366).
-- Player CastSpell takes (target, spellId[, triggered]) — the
-- triggered flag rides the in-tree 3-arg form (kalecgos :1052,
-- illidari council :403 precedent); the hit player is included
-- in the group loop exactly as in C++ (within 20 yd of self).

local SPELL_TELEPORT_SINGLE = 12885
local SPELL_TELEPORT_SINGLE_IN_GROUP = 13142
local SPELL_TELEPORT_GROUP = 27686

local function razelikhTeleportGroup(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    if target:GetObjectType() ~= "Player" then
        return
    end
    local group = target:GetGroup()
    if group then
        for _, member in ipairs(group:GetMembers()) do
            if not member:IsDead() and member:GetDistance(target) - 1.5 - 1.5 < 20 then
                member:CastSpell(member, SPELL_TELEPORT_SINGLE_IN_GROUP, true)
            end
        end
    else
        target:CastSpell(target, SPELL_TELEPORT_SINGLE, true)
    end
end

RegisterSpellEvent(SPELL_TELEPORT_GROUP, 5, razelikhTeleportGroup)
