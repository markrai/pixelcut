local handlers,bindings,props={},{},{}
local timer
local payload
local mp={}
function mp.create_osd_overlay() return {update=function() end,remove=function() end} end
function mp.set_property_native(k,v) props[k]=v;return true end
function mp.set_property(k,v) props[k]=v;return true end
function mp.get_property(k) return props[k] end
function mp.commandv(command,op,value) assert(command=="vf" and value~=nil);props["vf"]=value;return true end
function mp.set_property_bool(k,v) props[k]=v;return true end
function mp.get_property_native(k) return props[k] end
function mp.get_property_number(k,default) return props[k] or default end
function mp.register_script_message(k,v) handlers[k]=v end
function mp.add_forced_key_binding(k,n,f) bindings[n]=f end
function mp.remove_key_binding(n) bindings[n]=nil end
function mp.add_periodic_timer(_,f) timer=f end
package.preload["mp"]=function() return mp end
package.preload["mp.utils"]=function() return {parse_json=function() return payload end} end
dofile("cmd/pixelcut-gui/crop.lua")
local function request(rotation,edit)
    props["video-params/rotate"]=rotation
    payload={serial=1,width=1080,height=1920,crop={x=40,y=80,width=1000,height=1760},edit=edit}
    handlers["pixelcut-crop"]("fixture")
end
for _,rotation in ipairs({0,90,180,270}) do
    request(rotation,false)
    assert(not props["user-data/pixelcut/crop"].ready)
    local graph="lavfi=[crop=1000:1760:40:80],format=rotate=0"
    payload.filters={[tostring(rotation)]=graph}
    props["video-params/colorlevels"]="full"
    handlers["pixelcut-crop"]("fixture")
    assert(props["vf"]==graph and props["video-crop"]=="")
    assert(props["user-data/pixelcut/crop"].ready)
end
request(0,false)
payload.crop={x=0,y=0,width=1080,height=1920}
handlers["pixelcut-crop"]("fixture")
assert(props["vf"]=="" and props["video-crop"]=="")
props["osd-dimensions"]={w=1080,h=1920,ml=0,mr=0,mt=0,mb=0}
request(0,true)
assert(props["video-crop"]=="")
props["mouse-pos"]={x=40,y=80}
bindings["pc-crop-mouse"]({event="down"})
props["mouse-pos"]={x=80,y=120}
timer()
bindings["pc-crop-mouse"]({event="up"})
local state=props["user-data/pixelcut/crop"]
assert(state.crop.x==80 and state.crop.y==120 and state.crop.width==960 and state.crop.height==1720)
assert(not state.dragging and state.ready)
bindings["pc-crop-enter"]()
assert(props["user-data/pixelcut/crop"].action=="apply")
print("Visual crop script tests passed")

request(90,false)
props["video-params/colorlevels"]="limited"
payload.filters={["90"]="lavfi=[transpose=clock,scale=120:160,pad=240:320:60:80:color=black,setsar=1],format=rotate=0"}
handlers["pixelcut-crop"]("fixture")
assert(props["vf"]==payload.filters["90"] and props["video-crop"]=="")
request(90,true)
assert(props["vf"]=="" and props["video-crop"]=="")
props["video-params/colorlevels"]="full"
payload.edit=false;payload.limited_only=true;payload.filters={["90"]="test"}
handlers["pixelcut-crop"]("fixture")
assert(not props["user-data/pixelcut/crop"].ready)
print("Transform preview script tests passed")
